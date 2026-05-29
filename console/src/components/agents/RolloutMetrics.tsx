'use client';

import React from 'react';

export interface RolloutMetric {
    label: string;
    value: number;
    unit: string;
    threshold?: number;
    status: 'healthy' | 'warning' | 'critical';
}

export interface RolloutMetricsProps {
    metrics: RolloutMetric[];
}

function MetricStatusDot({ status }: { status: RolloutMetric['status'] }) {
    const color =
        status === 'healthy' ? 'bg-green-500' :
            status === 'warning' ? 'bg-yellow-500' :
                'bg-red-500';
    return <span className={`inline-block h-2 w-2 rounded-full ${color}`} />;
}

export function RolloutMetrics({ metrics }: RolloutMetricsProps) {
    return (
        <div className="space-y-3">
            <h3 className="text-sm font-semibold text-gray-700">Analysis Metrics</h3>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                {metrics.map((metric) => (
                    <div
                        key={metric.label}
                        className="rounded-lg border p-3"
                    >
                        <div className="flex items-center gap-2">
                            <MetricStatusDot status={metric.status} />
                            <span className="text-xs text-gray-500">{metric.label}</span>
                        </div>
                        <p className="mt-1 text-lg font-semibold">
                            {metric.value}
                            <span className="ml-0.5 text-sm font-normal text-gray-400">{metric.unit}</span>
                        </p>
                        {metric.threshold != null && (
                            <p className="text-xs text-gray-400">
                                Threshold: {metric.threshold}{metric.unit}
                            </p>
                        )}
                    </div>
                ))}
            </div>
        </div>
    );
}
