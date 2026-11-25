import {useState} from "react";
import {fallbackRuns,type FallbackRun} from "./api";
import {ObservationDetail} from "./History";

export function Fallbacks({monitor}:{monitor:string}) {
 const [page,setPage]=useState<{items:FallbackRun[];nextCursor:string|null}|null>(null);
 const [busy,setBusy]=useState(false),[error,setError]=useState(""),[selected,setSelected]=useState<string|null>(null);
 async function load(cursor:string|null){setBusy(true);setError("");try{setPage(await fallbackRuns(monitor,cursor));}catch(e){setError(e instanceof Error?e.message:"Історія недоступна");}finally{setBusy(false);}}
 return <section aria-busy={busy}><h2>Резервні спроби виконання</h2><p>Лише технічні помилки виконавця. Результат іншої зони не відновлює покриття початкової; початкові докази збережено окремо.</p>
 <button disabled={busy} onClick={()=>void load(null)}>Оновити резервні спроби</button>{error&&<p role="alert">{error}</p>}
 {page&&<><div className="table-scroll"><table><thead><tr><th>Початкове джерело / причина</th><th>Стратегія / стан</th><th>Окремі спроби та докази</th></tr></thead><tbody>{page.items.map(r=><tr key={r.id}>
 <td>{r.check}: {r.originAgent} / {r.originZone}<br/>{r.cause}<br/><small>{r.id}</small>{r.originObservationId?<p><button onClick={()=>setSelected(r.originObservationId)}>Початковий доказ</button></p>:<p>Початкове фізичне виконання не підтверджено; Observation не вигадано.</p>}</td>
 <td>{r.strategy}<br/>{r.state}<br/>{r.usedAttempts} / {r.maxAttempts} спроб<br/><small>{new Date(r.startedAt).toLocaleString("uk-UA")}</small></td>
 <td>{r.tasks.length?r.tasks.map(t=><p key={t.id}>#{t.attempt}: {t.agent} / {t.zone} · {t.state}<br/>{t.observationId?<button onClick={()=>setSelected(t.observationId)}>Доказ резервної спроби</button>:"Результат не отримано"}</p>):"Немає зарезервованих спроб"}</td>
 </tr>)}</tbody></table></div>{page.items.length===0&&<p>Резервних запусків немає.</p>}<button disabled={busy||!page.nextCursor} onClick={()=>void load(page.nextCursor)}>Наступна сторінка</button></>}
 {selected&&<ObservationDetail key={selected} id={selected} close={()=>setSelected(null)}/>}</section>;
}
