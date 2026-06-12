import type { CloudProviderType } from './types';

export type ProviderSecretKind = 'api-key' | 'service-account-json' | 'aws-credentials-json' | 'none';

export type CloudProviderSetup = {
  provider: CloudProviderType;
  titleKey: string;
  descriptionKey: string;
  secretKind: ProviderSecretKind;
  secretRequired: boolean;
  secretLabelKey?: string;
  secretPlaceholderKey?: string;
  baseUrlLabelKey?: string;
  baseUrlPlaceholder?: string;
  baseUrlRequired?: boolean;
  defaultBaseUrl?: string;
};

export const CLOUD_PROVIDER_SETUPS: CloudProviderSetup[] = [
  {
    provider: 'ollama',
    titleKey: 'cloud_providers.ollama.title',
    descriptionKey: 'cloud_providers.ollama.description',
    secretKind: 'api-key',
    secretRequired: false,
    secretLabelKey: 'cloud_providers.api_key_optional',
    secretPlaceholderKey: 'cloud_providers.api_key_placeholder_optional',
    baseUrlLabelKey: 'cloud_providers.base_url',
    baseUrlPlaceholder: 'http://127.0.0.1:11434 or https://ollama.com/api',
    defaultBaseUrl: 'http://127.0.0.1:11434',
  },
  {
    provider: 'openai',
    titleKey: 'cloud_providers.openai.title',
    descriptionKey: 'cloud_providers.openai.description',
    secretKind: 'api-key',
    secretRequired: true,
    secretLabelKey: 'cloud_providers.api_key',
    secretPlaceholderKey: 'cloud_providers.api_key_placeholder',
    baseUrlLabelKey: 'cloud_providers.base_url_optional',
    baseUrlPlaceholder: 'https://api.openai.com/v1',
    defaultBaseUrl: 'https://api.openai.com/v1',
  },
  {
    provider: 'openrouter',
    titleKey: 'cloud_providers.openrouter.title',
    descriptionKey: 'cloud_providers.openrouter.description',
    secretKind: 'api-key',
    secretRequired: true,
    secretLabelKey: 'cloud_providers.api_key',
    secretPlaceholderKey: 'cloud_providers.api_key_placeholder',
    baseUrlLabelKey: 'cloud_providers.base_url_optional',
    baseUrlPlaceholder: 'https://openrouter.ai/api/v1',
    defaultBaseUrl: 'https://openrouter.ai/api/v1',
  },
  {
    provider: 'anthropic',
    titleKey: 'cloud_providers.anthropic.title',
    descriptionKey: 'cloud_providers.anthropic.description',
    secretKind: 'api-key',
    secretRequired: true,
    secretLabelKey: 'cloud_providers.api_key',
    secretPlaceholderKey: 'cloud_providers.api_key_placeholder',
    baseUrlLabelKey: 'cloud_providers.base_url_optional',
    baseUrlPlaceholder: 'https://api.anthropic.com',
    defaultBaseUrl: 'https://api.anthropic.com',
  },
  {
    provider: 'gemini',
    titleKey: 'cloud_providers.gemini.title',
    descriptionKey: 'cloud_providers.gemini.description',
    secretKind: 'api-key',
    secretRequired: true,
    secretLabelKey: 'cloud_providers.api_key',
    secretPlaceholderKey: 'cloud_providers.api_key_placeholder',
    baseUrlLabelKey: 'cloud_providers.base_url_optional',
    baseUrlPlaceholder: 'https://generativelanguage.googleapis.com',
    defaultBaseUrl: 'https://generativelanguage.googleapis.com',
  },
  {
    provider: 'mistral',
    titleKey: 'cloud_providers.mistral.title',
    descriptionKey: 'cloud_providers.mistral.description',
    secretKind: 'api-key',
    secretRequired: true,
    secretLabelKey: 'cloud_providers.api_key',
    secretPlaceholderKey: 'cloud_providers.api_key_placeholder',
    baseUrlLabelKey: 'cloud_providers.base_url_optional',
    baseUrlPlaceholder: 'https://api.mistral.ai/v1',
    defaultBaseUrl: 'https://api.mistral.ai/v1',
  },
  {
    provider: 'vertex-google',
    titleKey: 'cloud_providers.vertex_google.title',
    descriptionKey: 'cloud_providers.vertex_google.description',
    secretKind: 'service-account-json',
    secretRequired: false,
    secretLabelKey: 'cloud_providers.service_account_json_optional',
    secretPlaceholderKey: 'cloud_providers.service_account_json_placeholder',
    baseUrlLabelKey: 'cloud_providers.vertex_url',
    baseUrlPlaceholder:
      'https://us-central1-aiplatform.googleapis.com/v1/projects/MY_PROJECT/locations/us-central1',
    baseUrlRequired: true,
  },
  {
    provider: 'bedrock',
    titleKey: 'cloud_providers.bedrock.title',
    descriptionKey: 'cloud_providers.bedrock.description',
    secretKind: 'aws-credentials-json',
    secretRequired: false,
    secretLabelKey: 'cloud_providers.aws_credentials_json_optional',
    secretPlaceholderKey: 'cloud_providers.aws_credentials_json_placeholder',
    baseUrlLabelKey: 'cloud_providers.bedrock_region',
    baseUrlPlaceholder: 'us-east-1',
    baseUrlRequired: true,
  },
];

export const ONBOARDING_PROVIDER_SETUPS = CLOUD_PROVIDER_SETUPS.filter(
  setup => setup.provider !== 'ollama',
);

export const getCloudProviderSetup = (provider: CloudProviderType): CloudProviderSetup =>
  CLOUD_PROVIDER_SETUPS.find(setup => setup.provider === provider) ?? CLOUD_PROVIDER_SETUPS[0];

export const isCloudProviderType = (provider: string): provider is CloudProviderType =>
  CLOUD_PROVIDER_SETUPS.some(setup => setup.provider === provider);
