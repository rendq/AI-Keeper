"use client";

import { useState } from "react";

export interface AuditEvent {
    timestamp: string;
    eventType: string;
    principal: string;
    action: string;
    decision: string;
    invocationId: string;
    outcome: string;
}

type SortField = keyof AuditEvent;
type SortDirection = "asc" | "desc";

interface AuditResultsTableProps {
    data: AuditEvent[];
    totalRows: number;
    page: number;
    pageSize: number;
    onPageChange: (page: number) => void;
    onExportCsv: () => void;
}

const COLUMNS: { key: SortField; label: string }[] = [
    { key: "timestamp", label: "Timestamp" },
    { key: "eventType", label: "Event Type" },
    { key: "principal", label: "Principal" },
    { key: "action", label: "Action" },
    { key: "decision", label: "Decision" },
    { key: "invocationId", label: "Invocation ID" },
    { key: "outcome", label: "Outcome" },
];

export function AuditResultsTable({
    data,
    totalRows,
    page,
    pageSize,
    onPageChange,
    onExportCsv,
}: AuditResultsTableProps) {
    const [sortField, setSortField] = useState<SortField>("timestamp");
    const [sortDirection, setSortDirection] = useState<SortDirection>("desc");

    const handleSort = (field: SortField) => {
        if (sortField === field) {
            setSortDirection(sortDirection === "asc" ? "desc" : "asc");
        } else {
            setSortField(field);
            setSortDirection("asc");
        }
    };

    const sortedData = [...data].sort((a, b) => {
        const aVal = a[sortField];
        const bVal = b[sortField];
        const cmp = aVal < bVal ? -1 : aVal > bVal ? 1 : 0;
        return sortDirection === "asc" ? cmp : -cmp;
    });

    const totalPages = Math.ceil(totalRows / pageSize);

    return (
        <div className="space-y-4">
            <div className="flex items-center justify-between">
                <p className="text-sm text-muted-foreground">
                    {totalRows} results (max 1000 rows)
                </p>
                <button
                    onClick={onExportCsv}
                    className="rounded-md border px-3 py-1.5 text-sm hover:bg-muted"
                >
                    Export CSV
                </button>
            </div>

            <div className="overflow-x-auto rounded-lg border">
                <table className="w-full text-sm">
                    <thead className="border-b bg-muted/50">
                        <tr>
                            {COLUMNS.map((col) => (
                                <th
                                    key={col.key}
                                    onClick={() => handleSort(col.key)}
                                    className="cursor-pointer px-3 py-2 text-left font-medium hover:bg-muted"
                                >
                                    {col.label}
                                    {sortField === col.key && (
                                        <span className="ml-1">
                                            {sortDirection === "asc" ? "↑" : "↓"}
                                        </span>
                                    )}
                                </th>
                            ))}
                        </tr>
                    </thead>
                    <tbody>
                        {sortedData.map((row, idx) => (
                            <tr key={idx} className="border-b last:border-0 hover:bg-muted/30">
                                <td className="px-3 py-2 font-mono text-xs">{row.timestamp}</td>
                                <td className="px-3 py-2">{row.eventType}</td>
                                <td className="px-3 py-2">{row.principal}</td>
                                <td className="px-3 py-2">{row.action}</td>
                                <td className="px-3 py-2">{row.decision}</td>
                                <td className="px-3 py-2 font-mono text-xs">{row.invocationId}</td>
                                <td className="px-3 py-2">{row.outcome}</td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>

            {totalPages > 1 && (
                <div className="flex items-center justify-center gap-2">
                    <button
                        onClick={() => onPageChange(page - 1)}
                        disabled={page <= 1}
                        className="rounded-md border px-3 py-1 text-sm disabled:opacity-50"
                    >
                        Previous
                    </button>
                    <span className="text-sm text-muted-foreground">
                        Page {page} of {totalPages}
                    </span>
                    <button
                        onClick={() => onPageChange(page + 1)}
                        disabled={page >= totalPages}
                        className="rounded-md border px-3 py-1 text-sm disabled:opacity-50"
                    >
                        Next
                    </button>
                </div>
            )}
        </div>
    );
}
