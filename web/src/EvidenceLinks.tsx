import { useState } from "react";
import { ObservationDetail } from "./History";

// Preserve the original evidence ID: never substitute a newer observation.
export function EvidenceLinks({ ids, canRead }: { ids: string[]; canRead: boolean }) {
  const [selected, setSelected] = useState<string | null>(null);
  return <>
    <ul>{ids.map(id => <li key={id}>
      {canRead && <><button type="button" onClick={() => setSelected(id)}>Відкрити доказ</button>{" "}</>}
      <code>{id}</code>
    </li>)}</ul>
    {canRead && selected && <ObservationDetail key={selected} id={selected} close={() => setSelected(null)} />}
  </>;
}
