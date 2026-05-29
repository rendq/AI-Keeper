"use client";

import { useState } from "react";
import { AuditQueryForm, type AuditQueryParams } from "@/components/audit/AuditQueryForm";
import { AuditResultsTable, type AuditEvent } from "@/components/audit/AuditResultsTable";

// Mock data for presentational stub
const MOCK_AUDIT_EVENTS: AuditEvent[] = [
    {
        timestamp: "2026-05-26T10:15:00Z",
        eventType: "policy.decision",
        principal: "sa:legal-copilot",
        action: "tool.invoke",
        decision: "allow",
        invocationId: "inv-abc-001",
        outcome: "success",
    },
    {
        timestamp: "2026-05-26T10:14:30Z",
        eventType: "policy.decision",
        principal: "sa:legal-copilot",
        action: "kb.query",
        decision: "allow",
        invocationId: "inv-abc-002",
        outcome: "success",
    },
    {
        timestamp: "2026-05-26T10:13:00Z",
        eventType: "policy.decision",
        principal: "sa:hr-bot",
        action: "tool.invoke",
        decision: "deny",
        invocationId: "inv-def-003",
        outcome: "blocked",
    },
    {
        timestamp: "2026-05-26T10:12:00Z",
        eventType: "budget.exceeded",
        principal: "sa:marketing-agent",
        action: "model.call",
        decision: "deny",
        invocationId: "inv-ghi-004",
        outcome: "quota_exceeded",
    },
    {
        timestamp: "2026-05-26T10:11:00Z",
        eventType: "policy.decision",
        principal: "sa:legal-copilot",
        action: "model.call",
        decision: "allow",
        invocationId: "inv-abc-005",
        outcome: "success",
    },
];

const PAGE_SIZE = 50;
const MAX_ROWS = 1000;

export default function AuditPage() {
    const [results, setResults] = useState<AuditEvent[]>([]);
    const [loading, setLoading] = useState(false);
    const [page, setPage] = useState(1);
    const [hasQueried, setHasQueried] = useState(false);

    const handleQuery = (_params: AuditQueryParams) => {
        setLoading(true);
        // Simulate async query — in production this translates expression to ClickHouse SQL
        setTimeout(() => {
            setResults(MOCK_AUDIT_EVENTS);
            setPage(1);
            setHasQueried(true);
            setLoading(false);
        }, 300);
    };

    const handleExportCsv = () => {
        if (results.length === 0) return;
        const headers = ["timestamp", "eventType", "principal", "action", "decision", "invocationId", "outcome"];
        const csvRows = [
            headers.join(","),
            ...results.map((row) =>
                headers.map((h) => `"${row[h as keyof AuditEvent]}"`).join(",")
            ),
        ];
        const blob = new Blob([csvRows.join("\n")], { type: "text/csv" });
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = "audit-export.csv";
        a.click();
        URL.revokeObjectURL(url);
    };

    const totalRows = Math.min(results.length, MAX_ROWS);

    return (
        <main className="mx-auto max-w-7xl p-6">
            <h1 className="mb-6 text-2xl font-bold">Audit Query</h1>

            <AuditQueryForm onSubmit={handleQuery} loading={loading} />

            {hasQueried && (
                <div className="mt-6">
                    <AuditResultsTable
                        data={results}
                        totalRows={totalRows}
                        page={page}
                        pageSize={PAGE_SIZE}
                        onPageChange={setPage}
                        onExportCsv={handleExportCsv}
                    />
                </div>
            )}
        </main>
    );
}
