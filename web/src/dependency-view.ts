import { resourceCatalog } from "./api";

export interface DependencyReference {
  kind: "Target" | "Monitor";
  name: string;
}
export interface DependencyLink {
  name: string;
  derived?: boolean;
  declaredBy?: string;
  from: DependencyReference;
  to: DependencyReference;
  relation: "requires" | "routesThrough" | "hostedOn" | "resolvesVia" | "relatedTo";
  strength: "hard" | "soft";
  description: string;
}
export const referenceKey = (ref: DependencyReference) => `${ref.kind}/${ref.name}`;

function reference(value: unknown): DependencyReference {
  const ref = value as Partial<DependencyReference> | null;
  if (!ref || (ref.kind !== "Target" && ref.kind !== "Monitor") || typeof ref.name !== "string") {
    throw new Error("Не вдалося прочитати посилання у відповіді сервера.");
  }
  return { kind: ref.kind, name: ref.name };
}
// These are presentation defaults from DSL §15, not a second graph validator.
// Cycle checks, authorization, health and revisions remain on Controller.
export async function dependencyPage(cursor: string | null, signal: AbortSignal) {
  const page = await resourceCatalog("Dependency", cursor, signal);
  return {
    nextCursor: page.nextCursor,
    items: page.items.map(({ manifest }): DependencyLink => {
      const spec = manifest.spec;
      return {
        name: manifest.metadata.name,
        from: reference(spec.from),
        to: reference(spec.to),
        relation: (spec.relation ?? "requires") as DependencyLink["relation"],
        strength: (spec.strength ?? "soft") as DependencyLink["strength"],
        description: typeof spec.description === "string" ? spec.description : "",
      };
    }),
  };
}
