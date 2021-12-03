import React, { Dispatch, SetStateAction, useEffect, useRef, useState } from 'react';
import { VariableSuggestion } from '@grafana/data';
import { DataSourcePicker } from '@grafana/runtime';
import { Button, DataLinkInput, InlineField, InlineFieldRow, Input, InlineSwitch } from '@grafana/ui';
import { DataLinkConfig } from '../types';
import { usePrevious } from 'react-use';
import { uniqueId } from 'lodash';

type Props = {
  value: DataLinkConfig;
  onChange: (value: DataLinkConfig) => void;
  onDelete: () => void;
  suggestions: VariableSuggestion[];
};
export const DataLink = (props: Props) => {
  const { value, onChange, onDelete, suggestions } = props;
  const [showInternalLink, setShowInternalLink] = useInternalLink(value.datasourceUid);
  const { current: baseId } = useRef(uniqueId('es-datalink-'));

  const handleChange = (field: keyof typeof value) => (event: React.ChangeEvent<HTMLInputElement>) => {
    onChange({
      ...value,
      [field]: event.currentTarget.value,
    });
  };

  return (
    <>
      <InlineFieldRow>
        <InlineField
          label="Field"
          tooltip="Can be exact field name or a regex pattern that will match on the field name."
          labelWidth={12}
          grow
        >
          <Input id={`${baseId}-field`} value={value.field} onChange={handleChange('field')} />
        </InlineField>
        <Button variant="destructive" aria-label="Remove field" icon="times" type="button" onClick={onDelete} />
      </InlineFieldRow>

      <InlineFieldRow>
        <InlineField
          label={showInternalLink ? 'Query' : 'URL'}
          tooltip="Can be exact field name or a regex pattern that will match on the field name."
          labelWidth={12}
          grow
        >
          <DataLinkInput
            placeholder={showInternalLink ? '${__value.raw}' : 'http://example.com/${__value.raw}'}
            value={value.url || ''}
            onChange={(newValue) =>
              onChange({
                ...value,
                url: newValue,
              })
            }
            suggestions={suggestions}
          />
        </InlineField>

        <InlineField label="URL Label" tooltip="Use to override the button label." labelWidth={12}>
          <Input id={`${baseId}-url-label`} value={value.urlDisplayLabel} onChange={handleChange('urlDisplayLabel')} />
        </InlineField>
      </InlineFieldRow>

      <InlineFieldRow>
        <InlineField labelWidth={12} label="Internal Link">
          <InlineSwitch
            id={`${baseId}-internal-link`}
            checked={showInternalLink}
            onChange={() => {
              if (showInternalLink) {
                onChange({
                  ...value,
                  datasourceUid: undefined,
                });
              }
              setShowInternalLink(!showInternalLink);
            }}
          />
        </InlineField>
        {showInternalLink && (
          <DataSourcePicker
            tracing={true}
            // Uid and value should be always set in the db and so in the items.
            onChange={(ds) => {
              onChange({
                ...value,
                datasourceUid: ds.uid,
              });
            }}
            current={value.datasourceUid}
          />
        )}
      </InlineFieldRow>
    </>
  );
};

function useInternalLink(datasourceUid?: string): [boolean, Dispatch<SetStateAction<boolean>>] {
  const [showInternalLink, setShowInternalLink] = useState<boolean>(!!datasourceUid);
  const previousUid = usePrevious(datasourceUid);

  // Force internal link visibility change if uid changed outside of this component.
  useEffect(() => {
    if (!previousUid && datasourceUid && !showInternalLink) {
      setShowInternalLink(true);
    }
    if (previousUid && !datasourceUid && showInternalLink) {
      setShowInternalLink(false);
    }
  }, [previousUid, datasourceUid, showInternalLink]);

  return [showInternalLink, setShowInternalLink];
}
