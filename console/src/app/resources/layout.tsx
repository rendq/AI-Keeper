import React from 'react';
import Link from 'next/link';
import { RESOURCE_KINDS, RESOURCE_SLUG } from '@/types/resources';

export default function ResourcesLayout({ children }: { children: React.ReactNode }) {
    return (
        <div className="flex min-h-screen">
            {/* Sidebar */}
            <aside className="w-56 border-r bg-gray-50 p-4">
                <Link href="/resources" className="mb-4 block text-lg font-bold text-gray-900">
                    Resources
                </Link>
                <nav className="space-y-1">
                    {RESOURCE_KINDS.map((kind) => (
                        <Link
                            key={kind}
                            href={`/resources/${RESOURCE_SLUG[kind]}`}
                            className="block rounded px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-200"
                        >
                            {kind}
                        </Link>
                    ))}
                </nav>
            </aside>

            {/* Main content */}
            <main className="flex-1 p-6">{children}</main>
        </div>
    );
}
