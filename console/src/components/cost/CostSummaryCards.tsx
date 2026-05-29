"use client";

export interface CostSummary {
    totalTokens: number;
    totalUsd: number;
    activeTenants: number;
    activeAgents: number;
}

interface CostSummaryCardsProps {
    summary: CostSummary;
}

function formatTokens(n: number): string {
    if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
    if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
    return n.toString();
}

export function CostSummaryCards({ summary }: CostSummaryCardsProps) {
    const cards = [
        { label: "Total Tokens", value: formatTokens(summary.totalTokens) },
        { label: "Total Cost (USD)", value: `$${summary.totalUsd.toFixed(2)}` },
        { label: "Active Tenants", value: summary.activeTenants.toString() },
        { label: "Active Agents", value: summary.activeAgents.toString() },
    ];

    return (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            {cards.map((card) => (
                <div
                    key={card.label}
                    className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm"
                >
                    <p className="text-sm text-gray-500">{card.label}</p>
                    <p className="mt-1 text-2xl font-semibold text-gray-900">
                        {card.value}
                    </p>
                </div>
            ))}
        </div>
    );
}
