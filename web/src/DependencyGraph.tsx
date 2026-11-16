import { useId, type KeyboardEvent } from "react";
import { referenceKey, type DependencyLink, type DependencyReference } from "./dependency-view";

interface Node extends DependencyReference { key: string; x: number; y: number }
interface Props {
  links: DependencyLink[];
  selectedNode: string;
  selectedLink: string;
  onNode: (key: string) => void;
  onLink: (name: string) => void;
}
const activate = (event: KeyboardEvent<SVGGElement>, action: () => void) => {
  if (event.key === "Enter" || event.key === " ") { event.preventDefault(); action(); }
};
export function DependencyGraph({ links, selectedNode, selectedLink, onNode, onLink }: Props) {
  const id = "dependency-" + useId().replace(/[^a-zA-Z0-9_-]/g, "_");
  const refs = new Map<string, DependencyReference>();
  for (const link of links) for (const ref of [link.from, link.to]) refs.set(referenceKey(ref), ref);
  const counts = { Monitor: 0, Target: 0 };
  const nodes: Node[] = [...refs.entries()].sort(([a], [b]) => a.localeCompare(b)).map(([key, ref]) => ({
    ...ref, key, x: ref.kind === "Monitor" ? 200 : 800, y: 65 + counts[ref.kind]++ * 90,
  }));
  const positions = new Map(nodes.map(node => [node.key, node]));
  const height = Math.max(250, 90 + Math.max(counts.Monitor, counts.Target) * 90);
  function path(link: DependencyLink, index: number) {
    const from = positions.get(referenceKey(link.from))!;
    const to = positions.get(referenceKey(link.to))!;
    if (from.kind === to.kind) {
      const x = from.kind === "Monitor" ? 90 : 910;
      const lane = from.kind === "Monitor" ? 15 + (index % 4) * 12 : 985 - (index % 4) * 12;
      return `M ${x} ${from.y} C ${lane} ${from.y}, ${lane} ${to.y}, ${x} ${to.y}`;
    }
    const start = from.kind === "Monitor" ? 310 : 690;
    const end = to.kind === "Monitor" ? 310 : 690;
    const lane = 450 + (index % 6) * 20;
    return `M ${start} ${from.y} C ${lane} ${from.y}, ${lane} ${to.y}, ${end} ${to.y}`;
  }
  return <div className="dependency-graph-scroll" tabIndex={0} aria-label="Прокручуваний граф залежностей">
    <svg viewBox={`0 0 1000 ${height}`} width="1000" height={height} role="group" aria-label="Граф залежностей поточної сторінки">
      <title>Стрілка від ресурсу, який залежить, до ресурсу залежності</title>
      <defs>
        <marker id={id + "-arrow"} viewBox="0 0 10 10" refX="10" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="#365b45" /></marker>
        <marker id={id + "-selected-arrow"} viewBox="0 0 10 10" refX="10" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="#875400" /></marker>
      </defs>
      {links.map((link, i) => <g key={link.name} role="button" tabIndex={0} aria-label={`Зв’язок ${link.name}`} aria-pressed={selectedLink === link.name}
        onClick={() => onLink(link.name)} onKeyDown={event => activate(event, () => onLink(link.name))} className="dependency-link">
        <title>{`${link.name}: ${referenceKey(link.from)} → ${referenceKey(link.to)}; ${link.relation}, ${link.strength}`}</title>
        <path className="dependency-edge-hit" d={path(link, i)} fill="none" stroke="transparent" strokeWidth="18" />
        <path className="dependency-edge" d={path(link, i)} fill="none" stroke={selectedLink === link.name ? "#875400" : "#365b45"} strokeWidth={selectedLink === link.name ? 4 : 2}
          strokeDasharray={link.relation === "requires" ? undefined : "7 5"} markerEnd={`url(#${id}${selectedLink === link.name ? "-selected-arrow" : "-arrow"})`} />
      </g>)}
      {nodes.map(node => <g key={node.key} role="button" tabIndex={0} aria-label={`Вузол ${node.key}`} aria-pressed={selectedNode === node.key}
        className="dependency-node" onClick={() => onNode(node.key)} onKeyDown={event => activate(event, () => onNode(node.key))}>
        <title>{node.key}</title>
        <rect x={node.x - 110} y={node.y - 28} width="220" height="56" rx="8" fill={node.kind === "Monitor" ? "#e3f0e6" : "#fdf5e6"}
          stroke={selectedNode === node.key ? "#875400" : "#365b45"} strokeWidth={selectedNode === node.key ? 3 : 1.5} />
        <text x={node.x} y={node.y - 5} textAnchor="middle" fontSize="13">{node.kind}</text>
        <text x={node.x} y={node.y + 15} textAnchor="middle" fontSize="15">{node.name.length > 23 ? node.name.slice(0, 22) + "…" : node.name}</text>
      </g>)}
    </svg>
  </div>;
}
