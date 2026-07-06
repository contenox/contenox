import { z, defineCollection } from 'astro:content';
import { glob } from 'astro/loaders';

// All content is sourced from the runtime repo's docs/ tree — the website
// owns no content. Everything under docs/ publishes; cookbook/ and stories/
// keep their own top-level routes, the rest renders under /docs/. Frontmatter
// is optional; pages without a title fall back to their first heading.
const docsSchema = z.object({
  title: z.string().optional(),
  description: z.string().optional(),
  draft: z.boolean().optional(),
});

const docs = defineCollection({
  loader: glob({
    pattern: ['**/*.md', '!cookbook/**', '!stories/**'],
    base: '../docs',
  }),
  schema: docsSchema,
});

const cookbook = defineCollection({
  loader: glob({ pattern: '**/*.md', base: '../docs/cookbook' }),
  schema: docsSchema,
});

const stories = defineCollection({
  loader: glob({ pattern: '**/*.md', base: '../docs/stories' }),
  schema: docsSchema,
});

export const collections = { docs, cookbook, stories };
