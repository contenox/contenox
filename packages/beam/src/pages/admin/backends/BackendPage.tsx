// src/pages/admin/backends/index.tsx
import { Page, TabbedPage } from '@contenox/ui';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router-dom';
import {
  useModeldCapacity,
  useLoadModeld,
  useModeldModels,
  useModeldStatus,
  useUnloadModeld,
} from '../../../hooks/useModeldStatus';
import { useRuntimeBackendState } from '../../../hooks/useRuntimeBackendState';
import BackendsSection from './components/BackendsSection';
import CloudProvidersSection from './components/CloudProvidersSection';
import LocalRuntimeSection from './components/LocalRuntimeSection';
import RuntimeStateSection from './components/RuntimeStateSection';

const BACKEND_TAB_IDS = ['backends', 'cloud-providers', 'local-runtime', 'state'] as const;

export default function BackendsPage() {
  const { t } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();
  const rawTab = searchParams.get('tab') ?? 'backends';
  const activeTab = (BACKEND_TAB_IDS as readonly string[]).includes(rawTab) ? rawTab : 'backends';
  const runtime = useRuntimeBackendState();
  const modeld = useModeldStatus();
  const modeldModels = useModeldModels();
  const [selectedLocalModel, setSelectedLocalModel] = useState('');
  const modeldCapacity = useModeldCapacity(selectedLocalModel);
  const loadModeld = useLoadModeld();
  const unloadModeld = useUnloadModeld();

  useEffect(() => {
    if (selectedLocalModel) return;

    const active = modeld.data?.slot?.active;
    const activeID =
      active?.type && active?.modelName ? `${active.type}:${active.modelName}` : undefined;
    const models = modeldModels.data ?? [];
    if (activeID && models.some(model => model.id === activeID)) {
      setSelectedLocalModel(activeID);
      return;
    }
    if (models[0]?.id) {
      setSelectedLocalModel(models[0].id);
    }
  }, [modeld.data?.slot?.active, modeldModels.data, selectedLocalModel]);

  const tabs = [
    {
      id: 'backends',
      label: t('backends.manage_title'),
      content: <BackendsSection />,
    },
    {
      id: 'cloud-providers',
      label: t('cloud_providers.title'),
      content: <CloudProvidersSection />,
    },
    {
      id: 'local-runtime',
      label: t('state.local_runtime_tab'),
      content: (
        <LocalRuntimeSection
          data={modeld.data}
          isLoading={modeld.isLoading}
          isError={modeld.isError}
          isFetching={modeld.isFetching}
          errorMessage={modeld.error?.message}
          models={modeldModels.data ?? []}
          modelsLoading={modeldModels.isLoading}
          modelsErrorMessage={modeldModels.error?.message}
          selectedModelId={selectedLocalModel}
          onSelectModel={setSelectedLocalModel}
          capacity={modeldCapacity.data}
          capacityLoading={modeldCapacity.isLoading}
          capacityFetching={modeldCapacity.isFetching}
          capacityErrorMessage={modeldCapacity.error?.message}
          onLoad={(model, generation) => loadModeld.mutate({ model, expectedGeneration: generation })}
          isLoadingModel={loadModeld.isPending}
          loadErrorMessage={loadModeld.error?.message}
          onUnload={generation => unloadModeld.mutate(generation)}
          isUnloading={unloadModeld.isPending}
          unloadErrorMessage={unloadModeld.error?.message}
          onRefresh={() => {
            void modeld.refetch();
            void modeldModels.refetch();
            if (selectedLocalModel) {
              void modeldCapacity.refetch();
            }
          }}
        />
      ),
    },
    {
      id: 'state',
      label: t('state.runtime_tab'),
      content: (
        <RuntimeStateSection
          data={runtime.data}
          isLoading={runtime.isLoading}
          isError={runtime.isError}
          errorMessage={runtime.error?.message}
        />
      ),
    },
  ];

  return (
    <Page bodyScroll="auto" className="h-full">
      <TabbedPage
        tabs={tabs}
        className="h-full"
        mountActivePanelOnly
        activeTab={activeTab}
        onTabChange={id => {
          setSearchParams(
            prev => {
              const next = new URLSearchParams(prev);
              next.set('tab', id);
              return next;
            },
            { replace: true },
          );
        }}
      />
    </Page>
  );
}
