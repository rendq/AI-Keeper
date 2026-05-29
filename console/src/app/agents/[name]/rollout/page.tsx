'use client';

import React, { useState } from 'react';
import { RolloutSlider } from '@/components/agents/RolloutSlider';
import { RolloutMetrics, type RolloutMetric } from '@/components/agents/RolloutMetrics';

/** Mock rollout status data */
const mockRolloutStatus = {
    phase: 'Progressing' as const,
    canaryWeight: 30,
    stableVersion: 'v1.2.0',
    canaryVersion: 'v1.3.0',
    steps: [10, 30, 100],
    analysisInterval: '5m',
    startedAt: '2024-06-01T10:00:00Z',
};

/** Mock analysis metrics */
const mockMetrics: RolloutMetric[] = [
    { label: 'Error Rate', value: 0.12, unit: '%', threshold: 1.0, status: 'healthy' },
    { label: 'P95 Latency', value: 230, unit: 'ms', threshold: 500, status: 'healthy' },
    { label: 'Guardrail Triggers', value: 2, unit: '/hr', threshold: 10, status: 'healthy' },
];

export default function AgentRolloutPage() {
    const [canaryWeight, setCanaryWeight] = useState(mockRolloutStatus.canaryWeight);
    const [phase, setPhase] = useState(mockRolloutStatus.phase);

    const handleRollback = () => {
        setCanaryWeight(0);
        setPhase('Aborted');
    };

    const isRollbackDisabled = phase === 'Aborted' || canaryWeight === 0;

    return (
        <div className="mx-auto max-w-3xl space-y-8 p-6">
            {/* Header */}
            <div>
                <h1 className="text-2xl font-bold">Agent Rollout Control</h1>
                <p className="mt-1 text-sm text-gray-500">
                    Manage canary deployment for this agent
                </p>
            </div>

            {/* Rollout Status */}
            <section className="rounded-lg border p-4">
                <h2 className="mb-3 text-lg font-semibold">Rollout Status</h2>
                <dl className="grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
                    <dt className="font-medium text-gray-600">Phase</dt>
                    <dd>
                        <span className={`inline-block rounded px-2 py-0.5 text-xs font-medium ${phase === 'Progressing' ? 'bg-blue-100 text-blue-800' :
                                phase === 'Aborted' ? 'bg-red-100 text-red-800' :
                                    'bg-green-100 text-green-800'
                            }`}>
                            {phase}
                        </span>
                    </dd>
                    <dt className="font-medium text-gray-600">Canary Weight</dt>
                    <dd>{canaryWeight}%</dd>
                    <dt className="font-medium text-gray-600">Stable Version</dt>
                    <dd className="font-mono">{mockRolloutStatus.stableVersion}</dd>
                    <dt className="font-medium text-gray-600">Canary Version</dt>
                    <dd className="font-mono">{mockRolloutStatus.canaryVersion}</dd>
                    <dt className="font-medium text-gray-600">Analysis Interval</dt>
                    <dd>{mockRolloutStatus.analysisInterval}</dd>
                    <dt className="font-medium text-gray-600">Started At</dt>
                    <dd>{new Date(mockRolloutStatus.startedAt).toLocaleString()}</dd>
                </dl>
            </section>

            {/* Canary Slider */}
            <section className="rounded-lg border p-4">
                <RolloutSlider
                    value={canaryWeight}
                    onChange={setCanaryWeight}
                    steps={mockRolloutStatus.steps}
                    disabled={phase === 'Aborted'}
                />
            </section>

            {/* Rollback Button */}
            <section>
                <button
                    type="button"
                    onClick={handleRollback}
                    disabled={isRollbackDisabled}
                    className="rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-50"
                >
                    Rollback
                </button>
            </section>

            {/* Analysis Metrics */}
            <section className="rounded-lg border p-4">
                <RolloutMetrics metrics={mockMetrics} />
            </section>
        </div>
    );
}
