"use client";

export type CostDimension = "Tenant" | "Team" | "Agent" | "Skill" | "ModelEndpoint";

export interface CostBreakdownRow {
    name: string;
    tokens: number;
    usd: number;
    percentage: number;
}

interface CostBreakdownProps {
    dimension: CostDimension;
    rows: CostBreakdownRow[];
}

export function CostBreakdown({ dimension, rows }: CostBreakdownProps) {
    return (
        <div className="overflow-x-auto rounded-lg border border-gray-200 bg-white">
            <table className="min-w-full divide-y divide-gray-200">
                <thead className="bg-gray-50">
                    <tr>
                        <th className="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">
                            {dimension}
                        </th>
                        <th className="px-4 py-3 text-right text-xs font-medium uppercase text-gray-500">
                            Tokens
                        </th>
                        <th className="px-4 py-3 text-right text-xs font-medium uppercase text-gray-500">
                            USD
                        </th>
                        <th className="px-4 py-3 text-right text-xs font-medium uppercase text-gray-500">
                            % of Total
                        </th>
                    </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                    {rows.map((row) => (
                        <tr key={row.name} className="hover:bg-gray-50">
                            <td className="whitespace-nowrap px-4 py-3 text-sm font-medium text-gray-900">
                                {row.name}
                            </td>
                            <td className="whitespace-nowrap px-4 py-3 text-right text-sm text-gray-700">
                                {row.tokens.toLocaleString()}
                            </td>
                            <td className="whitespace-nowrap px-4 py-3 text-right text-sm text-gray-700">
                                ${row.usd.toFixed(2)}
                            </td>
                            <td className="whitespace-nowrap px-4 py-3 text-right text-sm text-gray-700">
                                {row.percentage.toFixed(1)}%
                            </td>
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    );
}
