/**
 * TypeScript types for all 12 AIP resource kinds.
 * Mirrors the CRD schema defined in api/ Go types.
 */

// ---------- Shared ----------

export type Phase = 'Pending' | 'Active' | 'Terminating' | 'Failed' | 'Suspended';

export interface Condition {
    type: string;
    status: 'True' | 'False' | 'Unknown';
    lastTransitionTime: string;
    reason: string;
    message: string;
}

export interface ObjectMeta {
    name: string;
    namespace: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    creationTimestamp: string;
}

export interface ResourceRef {
    group: string;
    kind: string;
    name: string;
    namespace?: string;
}

// ---------- Resource Kind Union ----------

export const RESOURCE_KINDS = [
    'Tenant',
    'ServiceAccount',
    'Skill',
    'Tool',
    'Agent',
    'Policy',
    'Budget',
    'Quota',
    'DataSource',
    'KnowledgeBase',
    'ModelEndpoint',
    'ModelRouter',
] as const;

export type ResourceKind = (typeof RESOURCE_KINDS)[number];

/** URL slug for each resource kind */
export const RESOURCE_SLUG: Record<ResourceKind, string> = {
    Tenant: 'tenants',
    ServiceAccount: 'serviceaccounts',
    Skill: 'skills',
    Tool: 'tools',
    Agent: 'agents',
    Policy: 'policies',
    Budget: 'budgets',
    Quota: 'quotas',
    DataSource: 'datasources',
    KnowledgeBase: 'knowledgebases',
    ModelEndpoint: 'modelendpoints',
    ModelRouter: 'modelrouters',
};

/** Reverse map: slug → kind */
export const SLUG_TO_KIND: Record<string, ResourceKind> = Object.fromEntries(
    Object.entries(RESOURCE_SLUG).map(([k, v]) => [v, k as ResourceKind]),
) as Record<string, ResourceKind>;

// ---------- Per-Kind Spec/Status ----------

export interface TenantSpec {
    displayName: string;
    adminRef: ResourceRef;
}

export interface ServiceAccountSpec {
    displayName: string;
    tenantRef: ResourceRef;
}

export interface SkillSpec {
    displayName: string;
    runtime: string;
    entrypoint: string;
}

export interface ToolSpec {
    displayName: string;
    protocol: string;
    endpoint: string;
}

export interface AgentSpec {
    displayName: string;
    skillRefs: ResourceRef[];
    modelRef: ResourceRef;
}

export interface PolicySpec {
    displayName: string;
    rules: Array<{ effect: 'Allow' | 'Deny'; actions: string[] }>;
}

export interface BudgetSpec {
    displayName: string;
    limit: string;
    period: string;
}

export interface QuotaSpec {
    displayName: string;
    hard: Record<string, string>;
}

export interface DataSourceSpec {
    displayName: string;
    type: string;
    connectionRef: ResourceRef;
}

export interface KnowledgeBaseSpec {
    displayName: string;
    dataSourceRefs: ResourceRef[];
    embeddingModel: string;
}

export interface ModelEndpointSpec {
    displayName: string;
    provider: string;
    model: string;
    endpoint: string;
}

export interface ModelRouterSpec {
    displayName: string;
    strategy: string;
    endpointRefs: ResourceRef[];
}

// ---------- Generic Resource ----------

export interface ResourceStatus {
    phase: Phase;
    conditions: Condition[];
    observedGeneration?: number;
}

export interface Resource<S = Record<string, unknown>> {
    apiVersion: string;
    kind: ResourceKind;
    metadata: ObjectMeta;
    spec: S;
    status: ResourceStatus;
}

// Typed resource aliases
export type TenantResource = Resource<TenantSpec>;
export type ServiceAccountResource = Resource<ServiceAccountSpec>;
export type SkillResource = Resource<SkillSpec>;
export type ToolResource = Resource<ToolSpec>;
export type AgentResource = Resource<AgentSpec>;
export type PolicyResource = Resource<PolicySpec>;
export type BudgetResource = Resource<BudgetSpec>;
export type QuotaResource = Resource<QuotaSpec>;
export type DataSourceResource = Resource<DataSourceSpec>;
export type KnowledgeBaseResource = Resource<KnowledgeBaseSpec>;
export type ModelEndpointResource = Resource<ModelEndpointSpec>;
export type ModelRouterResource = Resource<ModelRouterSpec>;

// ---------- List ----------

export interface ResourceList<S = Record<string, unknown>> {
    items: Resource<S>[];
    total: number;
}

// ---------- Filter ----------

export interface ResourceFilter {
    namespace?: string;
    labelSelector?: string;
    search?: string;
    page?: number;
    pageSize?: number;
}
