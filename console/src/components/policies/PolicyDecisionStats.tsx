"use client";

export interface DecisionStats {
    allow: number;
    deny: number;
    requireApproval: number;
}

interface PolicyDecisionStatsProps {
    decisions24h: DecisionStats;
}

export function PolicyDecisionStats({ decisions24h }: PolicyDecisionStatsProps) {
    const total = decisions24h.allow + decisions24h.deny + decisions24h.requireApproval;

    const segments = [
        { label: "Allow", count: decisions24h.allow, color: "bg-green-500" },
        { label: "Deny", count: decisions24h.deny, color: "bg-red-500" },
        { label: "Require Approval", count: decisions24h.requireApproval, color: "bg-yellow-500" },
    ];

    return (
        <div className="rounded-lg border border-gray-200 bg-white p-6">
            <h3 className="text-lg font-semibold text-gray-900">Decisions (24h)</h3>
            <p className="mt-1 text-sm text-gray-500">Total: {total}</p>

            {/* Donut chart placeholder */}
            <div className="mt-4 flex items-center justify-center">
                <div className="flex h-32 w-32 items-center justify-center rounded-full border-8 border-gray-100">
                    <span className="text-xl font-bold text-gray-700">{total}</span>
                </div>
            </div>

            {/* Bar breakdown */}
            <div className="mt-4 space-y-2">
                {segments.map((seg) => {
                    const pct = total > 0 ? (seg.count / total) * 100 : 0;
                    return (
                        <div key={seg.label}>
                            <div className="flex items-center justify-between text-sm">
                                <span className="text-gray-600">{seg.label}</span>
                                <span className="font-medium text-gray-900">
                                    {seg.count} ({pct.toFixed(1)}%)
                                </span>
                            </div>
                            <div className="mt-1 h-2 w-full rounded-full bg-gray-100">
                                <div
                                    className={`h-2 rounded-full ${seg.color}`}
                                    style={{ width: `${pct}%` }}
                                />
                            </div>
                        </div>
                    );
                })}
            </div>
        </div>
    );
}
