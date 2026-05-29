"use client";

import { useState } from "react";

export interface AuditQueryParams {
    expression: string;
    startDate: string;
    endDate: string;
    tenant: string;
    agent: string;
}

interface AuditQueryFormProps {
    onSubmit: (params: AuditQueryParams) => void;
    loading?: boolean;
}

export function AuditQueryForm({ onSubmit, loading }: AuditQueryFormProps) {
    const [expression, setExpression] = useState("");
    const [startDate, setStartDate] = useState("");
    const [endDate, setEndDate] = useState("");
    const [tenant, setTenant] = useState("");
    const [agent, setAgent] = useState("");

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault();
        onSubmit({ expression, startDate, endDate, tenant, agent });
    };

    return (
        <form onSubmit={handleSubmit} className="space-y-4 rounded-lg border p-4">
            <div>
                <label htmlFor="expression" className="block text-sm font-medium">
                    Query Expression
                </label>
                <textarea
                    id="expression"
                    value={expression}
                    onChange={(e) => setExpression(e.target.value)}
                    placeholder='e.g. eventType = "policy.decision" AND decision = "deny"'
                    className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
                    rows={3}
                />
            </div>

            <div className="grid grid-cols-2 gap-4">
                <div>
                    <label htmlFor="startDate" className="block text-sm font-medium">
                        Start Date
                    </label>
                    <input
                        id="startDate"
                        type="datetime-local"
                        value={startDate}
                        onChange={(e) => setStartDate(e.target.value)}
                        className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
                    />
                </div>
                <div>
                    <label htmlFor="endDate" className="block text-sm font-medium">
                        End Date
                    </label>
                    <input
                        id="endDate"
                        type="datetime-local"
                        value={endDate}
                        onChange={(e) => setEndDate(e.target.value)}
                        className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
                    />
                </div>
            </div>

            <div className="grid grid-cols-2 gap-4">
                <div>
                    <label htmlFor="tenant" className="block text-sm font-medium">
                        Tenant
                    </label>
                    <input
                        id="tenant"
                        type="text"
                        value={tenant}
                        onChange={(e) => setTenant(e.target.value)}
                        placeholder="Filter by tenant"
                        className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
                    />
                </div>
                <div>
                    <label htmlFor="agent" className="block text-sm font-medium">
                        Agent
                    </label>
                    <input
                        id="agent"
                        type="text"
                        value={agent}
                        onChange={(e) => setAgent(e.target.value)}
                        placeholder="Filter by agent"
                        className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
                    />
                </div>
            </div>

            <button
                type="submit"
                disabled={loading}
                className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
                {loading ? "Querying..." : "Run Query"}
            </button>
        </form>
    );
}
