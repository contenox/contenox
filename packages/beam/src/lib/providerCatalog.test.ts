import { describe, expect, it } from 'vitest';
import { CLOUD_PROVIDER_SETUPS, isCloudProviderType } from './providerCatalog';

const supportedProviderTypes = new Set([
  'ollama',
  'openai',
  'anthropic',
  'gemini',
  'mistral',
  'bedrock',
  'vertex-google',
]);

describe('providerCatalog', () => {
  it('only exposes provider ids accepted by the runtime provider service', () => {
    for (const setup of CLOUD_PROVIDER_SETUPS) {
      expect(supportedProviderTypes.has(setup.provider)).toBe(true);
    }
  });

  it('does not expose removed Vertex publisher-specific backend ids', () => {
    expect(isCloudProviderType('vertex-anthropic')).toBe(false);
    expect(isCloudProviderType('vertex-meta')).toBe(false);
    expect(isCloudProviderType('vertex-mistralai')).toBe(false);
  });
});

