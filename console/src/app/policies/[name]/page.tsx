"use client";

import { PolicyDecisionStats, DecisionStats } from "@/components/policies/PolicyDecisionStats";
import { PolicyEffectiveWindow, EffectiveWindow } from "@/components/policies/PolicyEffectiveWindow";

// ---------- Mock Data ----------

interface PolicyConflict {
    policyName: string;
    severity: "hard" | "soft";
    description: string;
}

interface PolicyDetail {
    name: string;
    effect: "Allow" | "Deny";
    priority: number;
    subjects: string[];
    resources: string[];
    conditions: string[];
    decisions24h: DecisionStats;
    conflicts: PolicyConflict[];
    effectiveWindow: EffectiveWindow;
}

const MOCK_POLICY: PolicyDetail = {
    name: "legal-acl",
    effect: "Allow",
    priority: 100,
    subjects: ["team:legal-ops", "agent:legal-copilot"],
    resources: ["knowledgebase:legal-kb", "tool:docusign-mcp"],
    conditions: ['request.classification <= "CONFIDENTIAL"', 'time.now().hour >= 8 && time.now().hour <= 20'],
    decisions24h: { allow: 182, deny: 14, requireApproval: 7 },
    conflicts: [
        {
            policyName: "global-deny-pii",
            severity: "soft",
            description: "Partial subject overlap with global-deny-pii on resource knowledgebase:legal-kb",
        },
    ],
    effectiveWindow: {
        notBefore: "2025-01-01T00:00:00Z",
        notAfter: "2025-12-31T23:59:59Z",
    },
};

// ---------- Page ----------

export default function PolicyDetailPage({ params }: { params: { name: string } }) {
    const policy = { ...MOCK_POLICY, name: params.name };

    return (
        <main className="mx-auto max-w-5xl space-y-8 p-8">
            {/* Header */}
            <div>
                <h1 className="text-3xl font-bold text-gray-900">Policy: {policy.name}</h1>
                <p className="mt-1 text-sm text-gray-500">
                    Effect: <span className="font-medium">{policy.effect}</span> &middot; Priority: {policy.priority}
                </p>
            </div>

            {/* Spec section */}
            <section className="rounded-lg border border-gray-200 bg-white p-6">
                <h2 className="text-lg font-semibold text-gray-900">Spec</h2>
                <div className="mt-4 grid grid-cols-1 gap-6 md:grid-cols-2">
                    <div>
                        <h4 className="text-sm font-medium text-gray-500">Subjects</h4>
                        <ul className="mt-1 list-inside list-disc text-sm text-gray-700">
                            {policy.subjects.map((s) => (
                                <li key={s}>{s}</li>
                            ))}
                        </ul>
                    </div>
                    <div>
                        <h4 className="text-sm font-medium text-gray-500">Resources</h4>
                        <ul className="mt-1 list-inside list-disc text-sm text-gray-700">
                            {policy.resources.map((r) => (
                                <li key={r}>{r}</li>
                            ))}
                        </ul>
                    </div>
                    <div className="md:col-span-2">
                        <h4 className="text-sm font-medium text-gray-500">Conditions (CEL)</h4>
                        <ul className="mt-1 space-y-1">
                            {policy.conditions.map((c) => (
                                <li key={c} className="rounded bg-gray-50 px-3 py-1 font-mono text-xs text-gray-800">
                                    {c}
                                </li>
                            ))}
                        </ul>
                    </div>
                </div>
            </section>

            {/* Stats + Effective Window */}
            <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
                <PolicyDecisionStats decisions24h={policy.decisions24h} />
                <PolicyEffectiveWindow window={policy.effectiveWindow} />
            </div>

            {/* Conflicts */}
            <section className="rounded-lg border border-gray-200 bg-white p-6">
                <h2 className="text-lg font-semibold text-gray-900">Conflicts</h2>
                {policy.conflicts.length === 0 ? (
                    <p className="mt-2 text-sm text-gray-500">No conflicts detected.</p>
                ) : (
                    <ul className="mt-4 space-y-3">
                        {policy.conflicts.map((conflict) => (
                            <li
                                key={conflict.policyName}
                                className="rounded-md border border-gray-100 bg-gray-50 p-4"
                            >
                                <div className="flex items-center gap-2">
                                    <span
                                        className={`inline-block rounded px-2 py-0.5 text-xs font-medium ${conflict.severity === "hard"
                                                ? "bg-red-100 text-red-700"
                                                : "bg-yellow-100 text-yellow-700"
                                            }`}
                                    >
                                        {conflict.severity}
                                    </span>
                                    <span className="text-sm font-medium text-gray-900">
                                        {conflict.policyName}
                                    </span>
                                </div>
                                <p className="mt-1 text-sm text-gray-600">{conflict.description}</p>
                            </li>
                        ))}
                    </ul>
                )}
            </section>
        </main>
    );
}
