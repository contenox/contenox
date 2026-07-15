import { Button, Checkbox, Dropdown, Select } from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import type { SessionConfigOption } from '../../../lib/acp';
import { flattenGroups, fromControlValue, toControlProps } from '../lib/configOptionMapping';

interface ControlProps {
  option: SessionConfigOption;
  onChange: (value: string | boolean) => void;
}

/** One control for one `SessionConfigOption`: a checkbox for booleans, a `<select>` for flat or grouped values (groups flattened as "Group / Value" labels — the shared `Select` has no `<optgroup>` support). */
function OneControl({ option, onChange }: ControlProps) {
  const props = toControlProps(option);

  if (props.kind === 'boolean') {
    return (
      <Checkbox
        label={option.name}
        checked={props.checked}
        onChange={e => onChange(fromControlValue('boolean', e.target.checked))}
      />
    );
  }

  const options = props.kind === 'select-grouped' ? flattenGroups(props.groups ?? []) : (props.options ?? []);

  return (
    <label className="flex min-w-0 items-center gap-2 text-sm">
      <span className="text-text-muted dark:text-dark-text-muted shrink-0 text-xs whitespace-nowrap">
        {option.name}
      </span>
      <Select
        aria-label={option.name}
        options={options}
        value={props.value}
        onChange={e => onChange(fromControlValue(props.kind, e.target.value))}
        className="min-w-0"
      />
    </label>
  );
}

export interface ConfigOptionControlsProps {
  configOptions: SessionConfigOption[];
  onChange: (configId: string, value: string | boolean) => void;
}

/**
 * One control per `session.configOptions` entry. Values come straight from
 * `session.configOptions` — which is replaced wholesale on every
 * `config_option_update` (see acpSessionState.ts) — so a change made from
 * another surface (or the agent itself) round-trips into these controls with
 * no extra plumbing here.
 *
 * Renders inline on `sm:` and wider; collapses into a `Dropdown` below that so
 * a handful of selects don't crowd a 375px header.
 */
export function ConfigOptionControls({ configOptions, onChange }: ConfigOptionControlsProps) {
  const { t } = useTranslation();
  if (configOptions.length === 0) return null;

  const controls = configOptions.map(opt => (
    <OneControl key={opt.id} option={opt} onChange={value => onChange(opt.id, value)} />
  ));

  return (
    <>
      <div className="hidden flex-wrap items-center gap-3 sm:flex">{controls}</div>
      <div className="sm:hidden">
        <Dropdown
          trigger={
            <Button type="button" variant="outline" palette="neutral" size="sm">
              {t('acp_chat.config_options_label')}
            </Button>
          }
          contentClassName="w-64 space-y-3 p-3"
        >
          {controls}
        </Dropdown>
      </div>
    </>
  );
}
