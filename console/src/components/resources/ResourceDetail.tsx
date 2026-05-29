'use client';

import React from 'react';
import type { Resource } from '@/types/resources';

export interface ResourceDetailProps {
    resource: Resource;
}

function ConditionBadge({ status }: { status: string }) {
    const color =
        status === 'True' ? 'bg-green-100 text-green-800' :
            status === 'False' ? 'bg-red-100 text-red-800' :
                'bg-yellow-100 text-yellow-800';
    return <span className={`inline-block rounded px-2 py-0.5 text-xs font-medium ${color}`}>{status}</span>;
}

export function ResourceDetail({ resource }: ResourceDetailProps) {
    const { metadata, spec, status } = resource;

    return (
        <div className="space-y-6">
            {/* Header */}
            <div>
                <h1 className="text-2xl font-bold">{metadata.name}</h1>
                <p className="text-sm text-gray-500">
                    {resource.kind} · {metadata.namespace} · Phase: {status.phase}
                </p>
            </div>

            {/* Metadata */}
            <section>
                <h2 className="mb-2 text-lg font-semibold">Metadata</h2>
                <dl className="grid grid-cols-2 gap-2 text-sm">
                    <dt className="font-medium text-gray-600">Namespace</dt>
                    <dd>{metadata.namespace}</dd>
                    <dt className="font-medium text-gray-600">Created</dt>
                    <dd>{metadata.creationTimestamp}</dd>
                    {metadata.labels && Object.keys(metadata.labels).length > 0 && (
                        <>
                            <dt className="font-medium text-gray-600">Labels</dt>
                            <dd className="flex flex-wrap gap-1">
                                {Object.entries(metadata.labels).map(([k, v]) => (
                                    <span key={k} className="rounded bg-gray-100 px-2 py-0.5 text-xs">
                                        {k}={v}
                                    </span>
                                ))}
                            </dd>
                        </>
                    )}
                </dl>
            </section>

            {/* Spec */}
            <section>
                <h2 className="mb-2 text-lg font-semibold">Spec</h2>
                <pre className="overflow-x-auto rounded bg-gray-50 p-4 text-xs">
                    {JSON.stringify(spec, null, 2)}
                </pre>
            </section>

            {/* Status & Conditions */}
            <section>
                <h2 className="mb-2 text-lg font-semibold">Status</h2>
                <p className="mb-2 text-sm">
                    Phase: <span className="font-medium">{status.phase}</span>
                    {status.observedGeneration != null && (
                        <span className="ml-4 text-gray-500">Generation: {status.observedGeneration}</span>
                    )}
                </p>
                {status.conditions.length > 0 && (
                    <div className="overflow-x-auto rounded border">
                        <table className="w-full text-left text-sm">
                            <thead className="border-b bg-gray-50">
                                <tr>
                                    <th className="px-3 py-2">Type</th>
                                    <th className="px-3 py-2">Status</th>
                                    <th className="px-3 py-2">Reason</th>
                                    <th className="px-3 py-2">Message</th>
                                    <th className="px-3 py-2">Last Transition</th>
                                </tr>
                            </thead>
                            <tbody>
                                {status.conditions.map((cond) => (
                                    <tr key={cond.type} className="border-b">
                                        <td className="px-3 py-2 font-medium">{cond.type}</td>
                                        <td className="px-3 py-2"><ConditionBadge status={cond.status} /></td>
                                        <td className="px-3 py-2">{cond.reason}</td>
                                        <td className="px-3 py-2">{cond.message}</td>
                                        <td className="px-3 py-2 text-gray-500">{cond.lastTransitionTime}</td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                )}
            </section>
        </div>
    );
}
