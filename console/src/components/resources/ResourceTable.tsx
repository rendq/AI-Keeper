'use client';

import React, { useState } from 'react';
import type { Resource, ResourceFilter, ResourceKind } from '@/types/resources';

/** Column definition for the resource table */
export interface ColumnDef {
    key: string;
    label: string;
    render?: (resource: Resource) => React.ReactNode;
}

/** Default columns shared across all resource types */
const DEFAULT_COLUMNS: ColumnDef[] = [
    { key: 'name', label: 'Name', render: (r) => r.metadata.name },
    { key: 'namespace', label: 'Namespace', render: (r) => r.metadata.namespace },
    { key: 'phase', label: 'Phase', render: (r) => r.status.phase },
    {
        key: 'age',
        label: 'Age',
        render: (r) => {
            const created = new Date(r.metadata.creationTimestamp);
            const now = new Date();
            const diffMs = now.getTime() - created.getTime();
            const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));
            if (diffDays > 0) return `${diffDays}d`;
            const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
            if (diffHours > 0) return `${diffHours}h`;
            const diffMins = Math.floor(diffMs / (1000 * 60));
            return `${diffMins}m`;
        },
    },
];

/** Extra columns per resource kind */
const KIND_COLUMNS: Partial<Record<ResourceKind, ColumnDef[]>> = {
    Skill: [{ key: 'runtime', label: 'Runtime', render: (r) => (r.spec as { runtime?: string }).runtime ?? '-' }],
    Agent: [{ key: 'model', label: 'Model', render: (r) => (r.spec as { modelRef?: { name: string } }).modelRef?.name ?? '-' }],
    ModelEndpoint: [{ key: 'provider', label: 'Provider', render: (r) => (r.spec as { provider?: string }).provider ?? '-' }],
    ModelRouter: [{ key: 'strategy', label: 'Strategy', render: (r) => (r.spec as { strategy?: string }).strategy ?? '-' }],
    DataSource: [{ key: 'type', label: 'Type', render: (r) => (r.spec as { type?: string }).type ?? '-' }],
    Budget: [{ key: 'limit', label: 'Limit', render: (r) => (r.spec as { limit?: string }).limit ?? '-' }],
};

export interface ResourceTableProps {
    kind: ResourceKind;
    resources: Resource[];
    filter: ResourceFilter;
    onFilterChange: (filter: ResourceFilter) => void;
    onRowClick?: (resource: Resource) => void;
}

export function ResourceTable({ kind, resources, filter, onFilterChange, onRowClick }: ResourceTableProps) {
    const [searchInput, setSearchInput] = useState(filter.search ?? '');

    const columns = [...DEFAULT_COLUMNS, ...(KIND_COLUMNS[kind] ?? [])];

    const handleSearchSubmit = (e: React.FormEvent) => {
        e.preventDefault();
        onFilterChange({ ...filter, search: searchInput, page: 1 });
    };

    return (
        <div className="space-y-4">
            {/* Filter bar */}
            <div className="flex items-center gap-4">
                <form onSubmit={handleSearchSubmit} className="flex gap-2">
                    <input
                        type="text"
                        placeholder="Search by name..."
                        value={searchInput}
                        onChange={(e) => setSearchInput(e.target.value)}
                        className="rounded border px-3 py-1.5 text-sm"
                        aria-label="Search resources"
                    />
                    <button type="submit" className="rounded bg-blue-600 px-3 py-1.5 text-sm text-white">
                        Search
                    </button>
                </form>
                <input
                    type="text"
                    placeholder="Namespace filter"
                    value={filter.namespace ?? ''}
                    onChange={(e) => onFilterChange({ ...filter, namespace: e.target.value || undefined, page: 1 })}
                    className="rounded border px-3 py-1.5 text-sm"
                    aria-label="Filter by namespace"
                />
            </div>

            {/* Table */}
            <div className="overflow-x-auto rounded border">
                <table className="w-full text-left text-sm">
                    <thead className="border-b bg-gray-50">
                        <tr>
                            {columns.map((col) => (
                                <th key={col.key} className="px-4 py-2 font-medium text-gray-700">
                                    {col.label}
                                </th>
                            ))}
                        </tr>
                    </thead>
                    <tbody>
                        {resources.length === 0 ? (
                            <tr>
                                <td colSpan={columns.length} className="px-4 py-8 text-center text-gray-500">
                                    No resources found
                                </td>
                            </tr>
                        ) : (
                            resources.map((resource) => (
                                <tr
                                    key={`${resource.metadata.namespace}/${resource.metadata.name}`}
                                    className="cursor-pointer border-b hover:bg-gray-50"
                                    onClick={() => onRowClick?.(resource)}
                                >
                                    {columns.map((col) => (
                                        <td key={col.key} className="px-4 py-2">
                                            {col.render ? col.render(resource) : '-'}
                                        </td>
                                    ))}
                                </tr>
                            ))
                        )}
                    </tbody>
                </table>
            </div>
        </div>
    );
}
