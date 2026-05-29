'use client';

import React, { useState } from 'react';
import { useRouter } from 'next/navigation';
import { ResourceTable } from '@/components/resources/ResourceTable';
import { MOCK_RESOURCES } from '@/lib/mock-resources';
import { SLUG_TO_KIND } from '@/types/resources';
import type { ResourceFilter } from '@/types/resources';

interface PageProps {
    params: { type: string };
}

/** Resource list page for a given resource type (dynamic route). */
export default function ResourceListPage({ params }: PageProps) {
    const router = useRouter();
    const kind = SLUG_TO_KIND[params.type];
    const [filter, setFilter] = useState<ResourceFilter>({});

    if (!kind) {
        return (
            <div className="py-8 text-center text-gray-500">
                Unknown resource type: <code>{params.type}</code>
            </div>
        );
    }

    // Apply client-side filtering on mock data
    let resources = MOCK_RESOURCES[kind] ?? [];
    if (filter.namespace) {
        resources = resources.filter((r) => r.metadata.namespace.includes(filter.namespace!));
    }
    if (filter.search) {
        const q = filter.search.toLowerCase();
        resources = resources.filter((r) => r.metadata.name.toLowerCase().includes(q));
    }

    return (
        <div>
            <h1 className="mb-4 text-2xl font-bold">{kind}</h1>
            <ResourceTable
                kind={kind}
                resources={resources}
                filter={filter}
                onFilterChange={setFilter}
                onRowClick={(r) => router.push(`/resources/${params.type}/${r.metadata.name}`)}
            />
        </div>
    );
}
