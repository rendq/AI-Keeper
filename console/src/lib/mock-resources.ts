/**
 * Mock data for resource list/detail pages.
 * Used as presentational stubs until real API integration.
 */

import type { Resource, ResourceKind } from '@/types/resources';

function makeMock(kind: ResourceKind, name: string, namespace: string, extra: Record<string, unknown> = {}): Resource {
    return {
        apiVersion: 'aip.io/v1alpha1',
        kind,
        metadata: {
            name,
            namespace,
            labels: { 'app.kubernetes.io/managed-by': 'aip-controller' },
            creationTimestamp: new Date(Date.now() - Math.random() * 7 * 24 * 60 * 60 * 1000).toISOString(),
        },
        spec: { displayName: name, ...extra },
        status: {
            phase: 'Active',
            conditions: [
                {
                    type: 'Ready',
                    status: 'True',
                    lastTransitionTime: new Date().toISOString(),
                    reason: 'Reconciled',
                    message: 'Resource is ready',
                },
            ],
            observedGeneration: 1,
        },
    };
}

export const MOCK_RESOURCES: Record<ResourceKind, Resource[]> = {
    Tenant: [
        makeMock('Tenant', 'acme-corp', 'aip-system', { adminRef: { group: 'core', kind: 'ServiceAccount', name: 'admin' } }),
        makeMock('Tenant', 'globex', 'aip-system', { adminRef: { group: 'core', kind: 'ServiceAccount', name: 'admin' } }),
    ],
    ServiceAccount: [
        makeMock('ServiceAccount', 'legal-copilot-sa', 'acme-corp', { tenantRef: { group: 'core', kind: 'Tenant', name: 'acme-corp' } }),
    ],
    Skill: [
        makeMock('Skill', 'contract-review', 'acme-corp', { runtime: 'python3.11', entrypoint: 'main.handler' }),
        makeMock('Skill', 'summarize', 'acme-corp', { runtime: 'python3.11', entrypoint: 'summarize.run' }),
    ],
    Tool: [
        makeMock('Tool', 'docusign-tool', 'acme-corp', { protocol: 'REST', endpoint: 'https://api.docusign.com' }),
    ],
    Agent: [
        makeMock('Agent', 'legal-copilot', 'acme-corp', {
            skillRefs: [{ group: 'skill', kind: 'Skill', name: 'contract-review' }],
            modelRef: { group: 'model', kind: 'ModelEndpoint', name: 'gpt4o-endpoint' },
        }),
    ],
    Policy: [
        makeMock('Policy', 'default-allow', 'acme-corp', { rules: [{ effect: 'Allow', actions: ['*'] }] }),
    ],
    Budget: [
        makeMock('Budget', 'monthly-10k', 'acme-corp', { limit: '$10000', period: '30d' }),
    ],
    Quota: [
        makeMock('Quota', 'team-quota', 'acme-corp', { hard: { 'requests.cpu': '100', 'requests.memory': '256Gi' } }),
    ],
    DataSource: [
        makeMock('DataSource', 'contracts-s3', 'acme-corp', { type: 'S3', connectionRef: { group: 'core', kind: 'Secret', name: 's3-creds' } }),
    ],
    KnowledgeBase: [
        makeMock('KnowledgeBase', 'legal-kb', 'acme-corp', {
            dataSourceRefs: [{ group: 'data', kind: 'DataSource', name: 'contracts-s3' }],
            embeddingModel: 'text-embedding-3-small',
        }),
    ],
    ModelEndpoint: [
        makeMock('ModelEndpoint', 'gpt4o-endpoint', 'acme-corp', { provider: 'OpenAI', model: 'gpt-4o', endpoint: 'https://api.openai.com/v1' }),
    ],
    ModelRouter: [
        makeMock('ModelRouter', 'primary-router', 'acme-corp', {
            strategy: 'cost-optimized',
            endpointRefs: [{ group: 'model', kind: 'ModelEndpoint', name: 'gpt4o-endpoint' }],
        }),
    ],
};
