// Package main is the entrypoint for the AIP controller manager.
//
// The binary registers schemes for every API group, wires the core
// reconcilers (Skill / Agent / Policy) onto a controller-runtime
// Manager, and exposes [setupWebhooks] / [setupReconcilers] hooks so
// unit tests can validate cross-controller informer plumbing without
// having to start the full Manager.
//
// The actual Manager bootstrap is gated behind the `--start` flag so
// the binary stays usable as a CI smoke test (`go run ./cmd/manager`
// prints scheme info and exits) while still being the production
// entrypoint when run with `--start`.
package main

import (
	"flag"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlmanager "sigs.k8s.io/controller-runtime/pkg/manager"

	agentv1alpha1 "github.com/ai-keeper/ai-keeper/api/agent/v1alpha1"
	auditv1alpha1 "github.com/ai-keeper/ai-keeper/api/audit/v1alpha1"
	corev1alpha1 "github.com/ai-keeper/ai-keeper/api/core/v1alpha1"
	datav1alpha1 "github.com/ai-keeper/ai-keeper/api/data/v1alpha1"
	modelv1alpha1 "github.com/ai-keeper/ai-keeper/api/model/v1alpha1"
	policyv1alpha1 "github.com/ai-keeper/ai-keeper/api/policy/v1alpha1"
	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	agentctrl "github.com/ai-keeper/ai-keeper/controllers/agent"
	budgetctrl "github.com/ai-keeper/ai-keeper/controllers/budget"
	datasourcectrl "github.com/ai-keeper/ai-keeper/controllers/datasource"
	knowledgebasectrl "github.com/ai-keeper/ai-keeper/controllers/knowledgebase"
	modelendpointctrl "github.com/ai-keeper/ai-keeper/controllers/modelendpoint"
	modelrouterctrl "github.com/ai-keeper/ai-keeper/controllers/modelrouter"
	policyctrl "github.com/ai-keeper/ai-keeper/controllers/policy"
	quotactrl "github.com/ai-keeper/ai-keeper/controllers/quota"
	serviceaccountctrl "github.com/ai-keeper/ai-keeper/controllers/serviceaccount"
	skillctrl "github.com/ai-keeper/ai-keeper/controllers/skill"
	tenantctrl "github.com/ai-keeper/ai-keeper/controllers/tenant"
	toolctrl "github.com/ai-keeper/ai-keeper/controllers/tool"
	conversionwiring "github.com/ai-keeper/ai-keeper/internal/conversion"
	webhookwiring "github.com/ai-keeper/ai-keeper/internal/webhook"
)

// scheme is the runtime.Scheme used by every controller wired into
// this binary. Built once at init() so subsequent reconcilers can
// reuse it directly.
var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(corev1alpha1.AddToScheme(scheme))
	utilruntime.Must(skillv1alpha1.AddToScheme(scheme))
	utilruntime.Must(agentv1alpha1.AddToScheme(scheme))
	utilruntime.Must(policyv1alpha1.AddToScheme(scheme))
	utilruntime.Must(datav1alpha1.AddToScheme(scheme))
	utilruntime.Must(modelv1alpha1.AddToScheme(scheme))
	utilruntime.Must(auditv1alpha1.AddToScheme(scheme))
}

func main() {
	var startManager bool
	flag.BoolVar(&startManager, "start", false,
		"Start the controller-runtime Manager (default: print scheme info and exit).")
	flag.Parse()

	if !startManager {
		fmt.Fprintln(os.Stdout, "aip-manager: scheme registered for 7 groups / 13 kinds; pass --start to run the Manager")
		return
	}

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	cfg, err := ctrl.GetConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "aip-manager: get kubeconfig: %v\n", err)
		os.Exit(1)
	}
	mgr, err := ctrl.NewManager(cfg, ctrlmanager.Options{Scheme: scheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "aip-manager: build manager: %v\n", err)
		os.Exit(1)
	}
	if err := setupReconcilers(mgr); err != nil {
		fmt.Fprintf(os.Stderr, "aip-manager: setup reconcilers: %v\n", err)
		os.Exit(1)
	}
	if err := setupWebhooks(mgr); err != nil {
		fmt.Fprintf(os.Stderr, "aip-manager: setup webhooks: %v\n", err)
		os.Exit(1)
	}
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		fmt.Fprintf(os.Stderr, "aip-manager: manager stopped with error: %v\n", err)
		os.Exit(1)
	}
}

// setupReconcilers registers the AIP reconcilers onto the supplied
// Manager and wires the cross-controller informer graph (task 3.6 /
// Requirement A6).
//
// Wiring summary:
//
//   - Skill controller watches Agent: every Agent create / update /
//     delete enqueues the referenced Skill so `status.referencingAgents`
//     stays current (Requirement A6.4 / A3.11).
//   - Agent controller watches Skill: status / version changes on a
//     Skill enqueue every Agent that references it
//     (Requirements A6.1, A6.2). Spec-body churn unrelated to version
//     is filtered out by the SkillStatusChangedPredicate.
//   - Policy controller does NOT watch Agent: PDP bundle distribution
//     is the contract surface, so a Policy spec change leaves Agent
//     Deployments in place (Requirement A6.3).
//   - Tenant controller (task 4.1) reconciles Tenant CRs into a
//     `tenant-<name>` Namespace + default Budget/Quota
//     (Requirement A7.1).
//   - ServiceAccount controller (task 4.1) registers each SA at the
//     Identity Broker and toggles RFC 8693 OBO when
//     `spec.allowOnBehalfOf=true` (Requirement A7.2).
//   - Tool controller (task 4.2) probes `spec.endpoint`, registers
//     the Tool in Tool_Registry, and gates approval on
//     `governance.sideEffects` (Requirement A7.3).
//   - DataSource controller (task 4.2) drives the connector adapter
//     and surfaces `Connected / DocumentCount / SizeBytes` (Requirement A7.4).
//   - KnowledgeBase controller (task 4.2) validates referenced
//     DataSources and runs the indexing pipeline placeholder
//     (Requirement A7.5 basic).
//   - ModelEndpoint controller (task 4.3) probes `spec.endpoint`,
//     records `currentTpm/currentRpm/errorRate24h/avgLatencyMs`, and
//     gates Ready on the DPA + WithinQuota conditions
//     (Requirement A7.6).
//   - ModelRouter controller (task 4.3) compiles `spec.rules` into a
//     runtime routing table, distributes it to Model_Router instances,
//     and degrades when every referenced ModelEndpoint is unreachable
//     (Requirement A7.7).
func setupReconcilers(mgr ctrlmanager.Manager) error {
	if mgr == nil {
		return fmt.Errorf("setupReconcilers: manager is nil")
	}
	skillR := &skillctrl.SkillReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	if err := skillR.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setupReconcilers: skill: %w", err)
	}
	agentR := &agentctrl.AgentReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	if err := agentR.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setupReconcilers: agent: %w", err)
	}
	policyR := &policyctrl.PolicyReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	if err := policyR.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setupReconcilers: policy: %w", err)
	}
	tenantR := &tenantctrl.TenantReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	if err := tenantR.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setupReconcilers: tenant: %w", err)
	}
	serviceAccountR := &serviceaccountctrl.ServiceAccountReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	if err := serviceAccountR.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setupReconcilers: serviceaccount: %w", err)
	}
	toolR := &toolctrl.ToolReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	if err := toolR.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setupReconcilers: tool: %w", err)
	}
	dataSourceR := &datasourcectrl.DataSourceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	if err := dataSourceR.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setupReconcilers: datasource: %w", err)
	}
	knowledgeBaseR := &knowledgebasectrl.KnowledgeBaseReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	if err := knowledgeBaseR.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setupReconcilers: knowledgebase: %w", err)
	}
	modelEndpointR := &modelendpointctrl.ModelEndpointReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	if err := modelEndpointR.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setupReconcilers: modelendpoint: %w", err)
	}
	modelRouterR := &modelrouterctrl.ModelRouterReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	if err := modelRouterR.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setupReconcilers: modelrouter: %w", err)
	}
	budgetR := &budgetctrl.BudgetReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	if err := budgetR.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setupReconcilers: budget: %w", err)
	}
	quotaR := &quotactrl.QuotaReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	if err := quotaR.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setupReconcilers: quota: %w", err)
	}
	return nil
}

// setupWebhooks registers every AIP ValidatingAdmissionWebhook with
// the supplied controller-runtime Manager, plus the ConversionWebhook
// echo handler from task 2.4.
//
// Validates: Requirements A1.3, A1.5, A2.1—A2.6, A11.1, A11.2 (placeholder).
func setupWebhooks(mgr ctrlmanager.Manager) error {
	if err := webhookwiring.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setupWebhooks: validating: %w", err)
	}
	if err := conversionwiring.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setupWebhooks: conversion: %w", err)
	}
	return nil
}
