'use client';

import React from 'react';
import Link from 'next/link';
import { ResourceDetail } from '@/components/resources/ResourceDetail';
import { MOCK_RESOURCES } from '@/lib/mock-resources';
import { SLUG_TO_KIND } from '@/types/resources';

interface PageProps {
    params: { type: string; name: string };
}

/** Resource detail page showing spec, status, and conditions. */
export default function ResourceDetailPage({ params }: PageProps) {
    const kind = SLUG_TO_KIND[params.type];

    if (!kind) {
        return (
            <div className="py-8 text-center text-gray-500">
                Unknown resource type: <code>{params.type}</code>
            </div>
        );
    }

    const resource = (MOCK_RESOURCES[kind] ?? []).find((r) => r.metadata.name === params.name);

    if (!resource) {
        return (
            <div className="py-8 text-center text-gray-500">
                Resource not found: <code>{params.name}</code>
            </div>
        );
    }

    return (
        <div>
            <Link href={`/resources/${params.type}`} className="mb-4 inline-block text-sm text-blue-600 hover:underline">
                ← Back to {kind} list
            </Link>
            <ResourceDetail resource={resource} />
        </div>
    );
}
