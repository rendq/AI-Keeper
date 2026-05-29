import React from 'react';
import Link from 'next/link';
import { RESOURCE_KINDS, RESOURCE_SLUG } from '@/types/resources';

/** Resource overview dashboard showing all 12 resource kinds. */
export default function ResourcesOverviewPage() {
    return (
        <div>
            <h1 className="mb-6 text-2xl font-bold">Resource Overview</h1>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
                {RESOURCE_KINDS.map((kind) => (
                    <Link
                        key={kind}
                        href={`/resources/${RESOURCE_SLUG[kind]}`}
                        className="rounded-lg border p-4 shadow-sm transition hover:shadow-md"
                    >
                        <h2 className="text-lg font-semibold">{kind}</h2>
                        <p className="mt-1 text-sm text-gray-500">Manage {kind} resources</p>
                    </Link>
                ))}
            </div>
        </div>
    );
}
