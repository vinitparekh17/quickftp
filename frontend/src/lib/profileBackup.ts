import { addToast } from './stores';

import { exportProfile, importProfile, type ProfileKind } from './wailsjs';

export async function transferProfile(
  kind: ProfileKind,
  refresh: () => Promise<void>,
  id?: number
) {
  const importing = id === undefined;
  try {
    if (!importing) {
      if (await exportProfile(kind, id)) {
        addToast(
          'success',
          kind === 'connection'
            ? 'Connection exported. The password is excluded.'
            : 'Project exported.'
        );
      }
      return;
    }
    const result = await importProfile(kind);
    if (!result) return;
    if (result.skipped) {
      addToast(
        'info',
        `A ${kind} with this name already exists. The existing profile was kept.`
      );
      return;
    }
    await refresh();
    addToast(
      'success',
      kind === 'connection'
        ? 'Connection imported. Open it to enter its password.'
        : 'Project imported. Review its local path and build command before deploying.'
    );
  } catch (err: any) {
    addToast(
      'error',
      `Could not ${importing ? 'import' : 'export'} ${kind}: ${err?.message || String(err)}`
    );
  }
}
