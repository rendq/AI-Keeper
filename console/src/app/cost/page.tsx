"use client";

import { useState } from "react";
import { CostSummaryCards, type CostSummary } from "@/components/cost/CostSummaryCards";
import { CostBreakdown, type CostDimension, type CostBreakdownRow } from "@/components/cost/CostBreakdown";

// --- Mock data ---

type Period = "today" | "7d" | "30d" | "custom";

const MOCK_SUMMARY: CostSummary = {
    totalTokens: 12_450_000,
    totalUsd: 847.32,
    activeTenants: 5,
    activeAgents: 14,
};

const MOCK_BREAKDOWN: Record<CostDimension, CostBreakdownRow[]> = {
    Tenant: [
        { name: "acme-corp", tokens: 5_200_000, usd: 354.10, percentage: 41.8 },
        { name: "globex-inc", tokens: 3_800_000, usd: 258.50, percentage: 30.5 },
        { name: "initech", tokens: 2_100_000, usd: 142.80, percentage: 16.9 },
        { name: "umbrella-co", tokens: 900_000, usd: 61.20, percentage: 7.2 },
        { name: "stark-labs", tokens: 450_000, usd: 30.72, percentage: 3.6 },
    ],
    Team: [
        { name: "engineering", tokens: 6_000_000, usd: 408.00, percentage: 48.2 },
        { name: "legal", tokens: 3_200_000, usd: 217.60, percentage: 25.7 },
        { name: "marketing", tokens: 2_000_000, usd: 136.00, percentage: 16.1 },
        { name: "hr", tokens: 1_250_000, usd: 85.72, percentage: 10.0 },
    ],
    Agent: [
        { name: "legal-copilot", tokens: 4_100_000, usd: 278.80, percentage: 32.9 },
        { name: "code-reviewer", tokens: 3_500_000, usd: 238.00, percentage: 28.1 },
        { name: "hr-bot", tokens: 2_200_000, usd: 149.60, percentage: 17.7 },
        { name: "marketing-agent", tokens: 1_600_000, usd: 108.80, percentage: 12.8 },
        { name: "data-analyst", tokens: 1_050_000, usd: 72.12, percentage: 8.5 },
    ],
    Skill: [
        { name: "document-search", tokens: 4_800_000, usd: 326.40, percentage: 38.5 },
        { name: "code-generation", tokens: 3_900_000, usd: 265.20, percentage: 31.3 },
        { name: "summarization", tokens: 2_400_000, usd: 163.20, percentage: 19.3 },
        { name: "translation", tokens: 1_350_000, usd: 92.52, percentage: 10.9 },
    ],
    ModelEndpoint: [
        { name: "gpt-4o (Azure East US)", tokens: 7_200_000, usd: 576.00, percentage: 57.8 },
        { name: "claude-3.5 (Bedrock)", tokens: 3_100_000, usd: 186.00, percentage: 24.9 },
        { name: "gpt-4o-mini (Azure)", tokens: 2_150_000, usd: 85.32, percentage: 17.3 },
    ],
};

// --- Page component ---

export default function CostPage() {
    const [dimension, setDimension] = useState<CostDimension>("Tenant");
    const [period, setPeriod] = useState<Period>("7d");

    const dimensions: CostDimension[] = ["Tenant", "Team", "Agent", "Skill", "ModelEndpoint"];
    const periods: { value: Period; label: string }[] = [
        { value: "today", label: "Today" },
        { value: "7d", label: "7 Days" },
        { value: "30d", label: "30 Days" },
        { value: "custom", label: "Custom" },
    ];

    return (
        <main className="mx-auto max-w-7xl p-6">
            <div className="mb-6 flex items-center justify-between">
                <h1 className="text-2xl font-bold">Cost Dashboard</h1>
                <div className="flex gap-1 rounded-md border border-gray-200 bg-white p-1">
                    {periods.map((p) => (
                        <button
                            key={p.value}
                            onClick={() => setPeriod(p.value)}
                            className={`rounded px-3 py-1 text-sm font-medium transition-colors ${period === p.value
                                    ? "bg-blue-600 text-white"
                                    : "text-gray-600 hover:bg-gray-100"
                                }`}
                        >
                            {p.label}
                        </button>
                    ))}
                </div>
            </div>

            {/* Summary cards */}
            <CostSummaryCards summary={MOCK_SUMMARY} />

            {/* Burn rate chart placeholder */}
            <div className="mt-6 flex h-48 items-center justify-center rounded-lg border border-dashed border-gray-300 bg-gray-50">
                <p className="text-sm text-gray-400">
                    📈 Burn Rate Chart — time-series visualization placeholder ({period})
                </p>
            </div>

            {/* Dimension selector + breakdown table */}
            <div className="mt-6">
                <div className="mb-4 flex gap-2">
                    {dimensions.map((d) => (
                        <button
                            key={d}
                            onClick={() => setDimension(d)}
                            className={`rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${dimension === d
                                    ? "bg-gray-900 text-white"
                                    : "border border-gray-200 bg-white text-gray-600 hover:bg-gray-100"
                                }`}
                        >
                            {d}
                        </button>
                    ))}
                </div>

                <CostBreakdown dimension={dimension} rows={MOCK_BREAKDOWN[dimension]} />
            </div>
        </main>
    );
}
