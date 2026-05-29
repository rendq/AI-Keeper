"use client";

import { useEffect, useState } from "react";

export interface EffectiveWindow {
    notBefore?: string;
    notAfter?: string;
}

interface PolicyEffectiveWindowProps {
    window: EffectiveWindow;
}

function formatCountdown(ms: number): string {
    if (ms <= 0) return "Expired";
    const seconds = Math.floor(ms / 1000) % 60;
    const minutes = Math.floor(ms / 60000) % 60;
    const hours = Math.floor(ms / 3600000) % 24;
    const days = Math.floor(ms / 86400000);
    if (days > 0) return `${days}d ${hours}h ${minutes}m`;
    if (hours > 0) return `${hours}h ${minutes}m ${seconds}s`;
    return `${minutes}m ${seconds}s`;
}

export function PolicyEffectiveWindow({ window }: PolicyEffectiveWindowProps) {
    const [now, setNow] = useState(Date.now());

    useEffect(() => {
        const timer = setInterval(() => setNow(Date.now()), 1000);
        return () => clearInterval(timer);
    }, []);

    const notBeforeMs = window.notBefore ? new Date(window.notBefore).getTime() : null;
    const notAfterMs = window.notAfter ? new Date(window.notAfter).getTime() : null;

    const isActive = (!notBeforeMs || now >= notBeforeMs) && (!notAfterMs || now < notAfterMs);
    const isSuspended = notBeforeMs !== null && now < notBeforeMs;
    const isExpired = notAfterMs !== null && now >= notAfterMs;

    let statusLabel: string;
    let statusColor: string;
    if (isExpired) {
        statusLabel = "Expired";
        statusColor = "text-red-600";
    } else if (isSuspended) {
        statusLabel = "Suspended (not yet active)";
        statusColor = "text-yellow-600";
    } else {
        statusLabel = "Active";
        statusColor = "text-green-600";
    }

    return (
        <div className="rounded-lg border border-gray-200 bg-white p-6">
            <h3 className="text-lg font-semibold text-gray-900">Effective Window</h3>

            <div className="mt-4 space-y-3">
                <div className="flex items-center justify-between">
                    <span className="text-sm text-gray-500">Status</span>
                    <span className={`text-sm font-semibold ${statusColor}`}>
                        {statusLabel}
                    </span>
                </div>

                {window.notBefore && (
                    <div className="flex items-center justify-between">
                        <span className="text-sm text-gray-500">Not Before</span>
                        <span className="text-sm text-gray-900">{window.notBefore}</span>
                    </div>
                )}

                {window.notAfter && (
                    <div className="flex items-center justify-between">
                        <span className="text-sm text-gray-500">Not After</span>
                        <span className="text-sm text-gray-900">{window.notAfter}</span>
                    </div>
                )}

                {isActive && notAfterMs && (
                    <div className="flex items-center justify-between">
                        <span className="text-sm text-gray-500">Expires In</span>
                        <span className="text-sm font-mono font-medium text-gray-900">
                            {formatCountdown(notAfterMs - now)}
                        </span>
                    </div>
                )}

                {isSuspended && notBeforeMs && (
                    <div className="flex items-center justify-between">
                        <span className="text-sm text-gray-500">Activates In</span>
                        <span className="text-sm font-mono font-medium text-gray-900">
                            {formatCountdown(notBeforeMs - now)}
                        </span>
                    </div>
                )}
            </div>
        </div>
    );
}
