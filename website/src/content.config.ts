import { z, defineCollection } from 'astro:content';
import { glob } from 'astro/loaders';

// All content is sourced from the runtime repo's docs/ tree — the website
// owns no content, and every page renders under /docs/. Frontmatter is
// optional; pages without a title fall back to their first heading.
const docsSchema = z.object({
  title: z.string().optional(),
  description: z.string().optional(),
  draft: z.boolean().optional(),
});

const docs = defineCollection({
  loader: glob({ pattern: '**/*.md', base: '../docs' }),
  schema: docsSchema,
});

export const collections = { docs };
