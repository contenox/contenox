import {
  Badge,
  Button,
  GridLayout,
  LoadingState,
  Panel,
  Section,
  Select,
  Span,
  Table,
  TableCell,
  TableRow,
} from '@contenox/ui';
import { Power, RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type {
  ModeldCapacityResponse,
  ModeldLocalModel,
  ModeldRuntimeConfig,
  ModeldSlotStatus,
  ModeldStatusResponse,
} from '../../../../lib/types';

type LocalRuntimeSectionProps = {
  data: ModeldStatusResponse | undefined;
  isLoading: boolean;
  isError: boolean;
  isFetching: boolean;
  errorMessage?: string;
  models: ModeldLocalModel[];
  modelsLoading: boolean;
  modelsErrorMessage?: string;
  selectedModelId: string;
  onSelectModel: (model: string) => void;
  capacity: ModeldCapacityResponse | undefined;
  capacityLoading: boolean;
  capacityFetching: boolean;
  capacityErrorMessage?: string;
  onUnload: (generation: number) => void;
  isUnloading: boolean;
  unloadErrorMessage?: string;
  onRefresh: () => void;
};

type BadgeVariant = 'default' | 'success' | 'error' | 'warning' | 'secondary' | 'outline';

type DetailRow = {
  label: string;
  value: string | number | boolean | number[] | undefined;
};

const missingValue = '-';

const statusVariant = (data: ModeldStatusResponse | undefined): BadgeVariant => {
  if (!data) return 'secondary';
  if (data.available) return 'success';
  if (data.state === 'stale' || data.state === 'unreachable') return 'error';
  return 'warning';
};

const detailValue = (value: DetailRow['value'], yes: string, no: string): string => {
  if (value === undefined || value === null || value === '') return missingValue;
  if (Array.isArray(value)) return value.length > 0 ? value.join(', ') : missingValue;
  if (typeof value === 'boolean') return value ? yes : no;
  return String(value);
};

const presentRows = (rows: DetailRow[]): DetailRow[] =>
  rows.filter(row => {
    if (row.value === undefined || row.value === null || row.value === '') return false;
    if (Array.isArray(row.value) && row.value.length === 0) return false;
    return true;
  });

const formatBytes = (value: number | undefined): string | undefined => {
  if (!value || value <= 0) return undefined;
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let next = value;
  let unit = 0;
  while (next >= 1024 && unit < units.length - 1) {
    next /= 1024;
    unit += 1;
  }
  return `${next >= 10 || unit === 0 ? next.toFixed(0) : next.toFixed(1)} ${units[unit]}`;
};

function DetailTable({ rows }: { rows: DetailRow[] }) {
  const { t } = useTranslation();
  return (
    <Table columns={[t('state.local_runtime_col_field'), t('state.local_runtime_col_value')]}>
      {rows.map(row => (
        <TableRow key={row.label}>
          <TableCell className="text-text-muted dark:text-dark-text-muted w-64">
            {row.label}
          </TableCell>
          <TableCell className="font-mono text-xs break-all">
            {detailValue(row.value, t('common.yes'), t('common.no'))}
          </TableCell>
        </TableRow>
      ))}
    </Table>
  );
}

function CapacityDetails({
  models,
  modelsLoading,
  modelsErrorMessage,
  selectedModelId,
  onSelectModel,
  capacity,
  capacityLoading,
  capacityFetching,
  capacityErrorMessage,
}: {
  models: ModeldLocalModel[];
  modelsLoading: boolean;
  modelsErrorMessage?: string;
  selectedModelId: string;
  onSelectModel: (model: string) => void;
  capacity: ModeldCapacityResponse | undefined;
  capacityLoading: boolean;
  capacityFetching: boolean;
  capacityErrorMessage?: string;
}) {
  const { t } = useTranslation();
  const options = models.map(model => ({
    value: model.id,
    label: `${model.model} (${model.backendType})`,
  }));
  const info = capacity?.info;
  const rows = info
    ? presentRows([
        { label: t('state.local_runtime_capacity_model_max'), value: info.modelMaxContext },
        { label: t('state.local_runtime_capacity_effective'), value: info.effectiveContext },
        {
          label: t('state.local_runtime_capacity_memory_context'),
          value: info.memoryContextTokens,
        },
        { label: t('state.local_runtime_cfg_hot_context'), value: info.hotContextTokens },
        {
          label: t('state.local_runtime_cfg_planner_context'),
          value: info.plannerEffectiveContext,
        },
        {
          label: t('state.local_runtime_capacity_kv_bytes'),
          value: formatBytes(info.kvBytesPerToken),
        },
        {
          label: t('state.local_runtime_capacity_free'),
          value: formatBytes(info.freeBytes),
        },
        {
          label: t('state.local_runtime_capacity_required'),
          value: formatBytes(info.requiredBytes),
        },
        {
          label: t('state.local_runtime_capacity_usable'),
          value: formatBytes(info.usableBytes),
        },
        {
          label: t('state.local_runtime_capacity_weights'),
          value: formatBytes(info.weightsBytes),
        },
        {
          label: t('state.local_runtime_capacity_host_cold'),
          value: formatBytes(info.hostColdBudgetBytes),
        },
        { label: t('state.local_runtime_capacity_clamped'), value: info.clamped },
        { label: t('state.local_runtime_capacity_reason'), value: info.reason },
        { label: t('state.local_runtime_capacity_device'), value: info.deviceKind },
        { label: t('state.local_runtime_capacity_device_id'), value: info.deviceId },
        {
          label: t('state.local_runtime_capacity_device_total'),
          value: formatBytes(info.deviceTotalBytes),
        },
        {
          label: t('state.local_runtime_capacity_gpu_offload'),
          value: info.supportsGpuOffload,
        },
        {
          label: t('state.local_runtime_capacity_requested_layers'),
          value: info.requestedGpuLayers,
        },
        {
          label: t('state.local_runtime_capacity_resolved_layers'),
          value: info.resolvedGpuLayers,
        },
        { label: t('state.local_runtime_capacity_runtime'), value: info.runtimeName },
        { label: t('state.local_runtime_capacity_runtime_digest'), value: info.runtimeDigest },
      ])
    : [];

  return (
    <Section title={t('state.local_runtime_capacity_title')}>
      <div className="space-y-4">
        {modelsErrorMessage && <Panel variant="error">{modelsErrorMessage}</Panel>}
        {models.length > 0 ? (
          <div className="max-w-md">
            <Select
              value={selectedModelId}
              onChange={event => onSelectModel(event.target.value)}
              options={options}
              disabled={modelsLoading || capacityFetching}
              className="w-full"
            />
          </div>
        ) : modelsLoading ? (
          <LoadingState />
        ) : (
          <Panel variant="empty" className="text-text-muted dark:text-dark-text-muted text-sm">
            {t('state.local_runtime_capacity_empty')}
          </Panel>
        )}

        {capacityErrorMessage && <Panel variant="error">{capacityErrorMessage}</Panel>}
        {capacityLoading && selectedModelId && <LoadingState />}
        {!capacityLoading && rows.length > 0 && <DetailTable rows={rows} />}
      </div>
    </Section>
  );
}

function SlotDetails({ slot }: { slot: ModeldSlotStatus }) {
  const { t } = useTranslation();
  const active = slot.active;
  const slotRows = presentRows([
    { label: t('state.local_runtime_slot_state'), value: slot.state },
    { label: t('state.local_runtime_backend'), value: slot.backend },
    { label: t('state.local_runtime_owner'), value: slot.ownerInstanceId },
    { label: t('state.local_runtime_busy_operation'), value: slot.busyOperation },
    { label: t('state.local_runtime_last_error'), value: slot.lastError },
    { label: t('state.local_runtime_active_model'), value: active?.modelName },
    { label: t('state.local_runtime_model_type'), value: active?.type },
    { label: t('state.local_runtime_digest'), value: active?.digest },
    { label: t('state.local_runtime_generation'), value: active?.generation },
  ]);

  return (
    <div className="space-y-4">
      <Section title={t('state.local_runtime_slot_title')}>
        {slotRows.length > 0 ? (
          <DetailTable rows={slotRows} />
        ) : (
          <Panel variant="empty" className="text-text-muted dark:text-dark-text-muted text-sm">
            {t('state.local_runtime_no_slot_desc')}
          </Panel>
        )}
      </Section>
      {active?.config && <ActiveConfigDetails config={active.config} />}
    </div>
  );
}

function ActiveConfigDetails({ config }: { config: ModeldRuntimeConfig }) {
  const { t } = useTranslation();
  const rows = presentRows([
    { label: t('state.local_runtime_cfg_num_ctx'), value: config.numCtx },
    { label: t('state.local_runtime_cfg_hot_context'), value: config.hotContextTokens },
    {
      label: t('state.local_runtime_cfg_planner_context'),
      value: config.plannerEffectiveContext,
    },
    { label: t('state.local_runtime_cfg_batch'), value: config.numBatch },
    { label: t('state.local_runtime_cfg_threads'), value: config.numThreads },
    { label: t('state.local_runtime_cfg_gpu_layers'), value: config.numGpuLayers },
    { label: t('state.local_runtime_cfg_tensor_split'), value: config.tensorSplit },
    { label: t('state.local_runtime_cfg_flash_attn'), value: config.flashAttn },
    { label: t('state.local_runtime_cfg_kv_cache'), value: config.kvCacheType },
    { label: t('state.local_runtime_cfg_prompt_format'), value: config.promptFormat },
    {
      label: t('state.local_runtime_cfg_prompt_digest'),
      value: config.promptTemplateDigest,
    },
    { label: t('state.local_runtime_cfg_disable_bos'), value: config.disableBOS },
    { label: t('state.local_runtime_cfg_reasoning'), value: config.reasoningFormat },
  ]);

  if (rows.length === 0) return null;

  return (
    <Section title={t('state.local_runtime_config_title')}>
      <DetailTable rows={rows} />
    </Section>
  );
}

export default function LocalRuntimeSection({
  data,
  isLoading,
  isError,
  isFetching,
  errorMessage,
  models,
  modelsLoading,
  modelsErrorMessage,
  selectedModelId,
  onSelectModel,
  capacity,
  capacityLoading,
  capacityFetching,
  capacityErrorMessage,
  onUnload,
  isUnloading,
  unloadErrorMessage,
  onRefresh,
}: LocalRuntimeSectionProps) {
  const { t } = useTranslation();
  const activeGeneration = data?.slot?.active?.generation;
  const statusRows = data
    ? presentRows([
        { label: t('state.local_runtime_daemon_state'), value: data.state },
        { label: t('state.local_runtime_backend'), value: data.backend },
        { label: t('state.local_runtime_endpoint'), value: data.endpoint },
        { label: t('state.local_runtime_owner'), value: data.instance },
        { label: t('state.local_runtime_binary'), value: data.binary },
        {
          label: t('state.local_runtime_protocol'),
          value: `${data.minRuntimeProtocol}-${data.runtimeProtocol}`,
        },
      ])
    : [];

  return (
    <div className="space-y-6">
      <Section title={t('state.local_runtime_title')} description={t('state.local_runtime_intro')}>
        <div className="flex justify-end gap-2">
          <Button
            type="button"
            variant="danger"
            size="sm"
            onClick={() => {
              if (activeGeneration !== undefined) onUnload(activeGeneration);
            }}
            isLoading={isUnloading}
            disabled={activeGeneration === undefined || isUnloading}
            className="gap-2">
            <Power className="h-4 w-4" aria-hidden="true" />
            {t('state.local_runtime_unload')}
          </Button>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={onRefresh}
            isLoading={isFetching}
            className="gap-2">
            <RefreshCw className="h-4 w-4" aria-hidden="true" />
            {t('state.local_runtime_refresh')}
          </Button>
        </div>

        {isLoading && <LoadingState />}
        {isError && (
          <Panel variant="error">{errorMessage || t('state.local_runtime_load_error')}</Panel>
        )}
        {unloadErrorMessage && <Panel variant="error">{unloadErrorMessage}</Panel>}
      </Section>

      {!isLoading && !isError && data && (
        <>
          <GridLayout variant="body" columns={3} responsive={{ base: 1, lg: 3 }} className="gap-4">
            <Panel variant="bordered" className="space-y-2">
              <Span variant="muted" className="block text-xs">
                {t('state.local_runtime_availability')}
              </Span>
              <Badge variant={statusVariant(data)} size="sm">
                {data.available
                  ? t('state.local_runtime_available')
                  : t('state.local_runtime_unavailable')}
              </Badge>
            </Panel>
            <Panel variant="bordered" className="space-y-2">
              <Span variant="muted" className="block text-xs">
                {t('state.local_runtime_daemon_state')}
              </Span>
              <Span className="font-mono text-sm">{data.state || missingValue}</Span>
            </Panel>
            <Panel variant="bordered" className="space-y-2">
              <Span variant="muted" className="block text-xs">
                {t('state.local_runtime_slot_state')}
              </Span>
              <Span className="font-mono text-sm">{data.slot?.state || missingValue}</Span>
            </Panel>
          </GridLayout>

          {data.error && (
            <Panel variant={data.available ? 'warning' : 'error'} className="text-sm">
              {data.error}
            </Panel>
          )}

          {statusRows.length > 0 && (
            <Section title={t('state.local_runtime_daemon_title')}>
              <DetailTable rows={statusRows} />
            </Section>
          )}

          {data.slot ? (
            <SlotDetails slot={data.slot} />
          ) : (
            <Section title={t('state.local_runtime_slot_title')}>
              <Panel variant="empty" className="text-text-muted dark:text-dark-text-muted text-sm">
                {t('state.local_runtime_no_slot_desc')}
              </Panel>
            </Section>
          )}

          <CapacityDetails
            models={models}
            modelsLoading={modelsLoading}
            modelsErrorMessage={modelsErrorMessage}
            selectedModelId={selectedModelId}
            onSelectModel={onSelectModel}
            capacity={capacity}
            capacityLoading={capacityLoading}
            capacityFetching={capacityFetching}
            capacityErrorMessage={capacityErrorMessage}
          />
        </>
      )}
    </div>
  );
}
