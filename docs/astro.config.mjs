import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

import cloudflare from '@astrojs/cloudflare';

export default defineConfig({
  site: 'https://docs.herald-mail.app',

  integrations: [
    starlight({
      title: 'Herald Docs',
      description: 'User and integration documentation for Herald, the terminal email client for power users.',
      social: [
        {
          icon: 'github',
          label: 'GitHub',
          href: 'https://github.com/herald-email/herald-mail-app',
        },
      ],
      sidebar: [
        {
          label: 'Getting Started',
          items: [
            { label: 'Overview', slug: '' },
            { label: 'Install and run', slug: 'getting-started' },
          ],
        },
        {
          label: 'Demo Mode',
          items: [{ label: 'Run without email', slug: 'demo-mode' }],
        },
        {
          label: 'Gmail Setup',
          items: [{ label: 'Gmail IMAP', slug: 'gmail-setup' }],
        },
        {
          label: 'Custom IMAP',
          items: [{ label: 'Provider settings', slug: 'custom-imap' }],
        },
        {
          label: 'MCP Setup',
          items: [{ label: 'AI tool integrations', slug: 'mcp-setup' }],
        },
        {
          label: 'Security & Privacy',
          items: [{ label: 'Local data model', slug: 'security-privacy' }],
        },
        {
          label: 'Troubleshooting',
          items: [{ label: 'Common issues', slug: 'troubleshooting' }],
        },
        {
          label: 'Uninstall',
          items: [{ label: 'Remove Herald', slug: 'uninstall' }],
        },
      ],
    }),
  ],

  adapter: cloudflare({
    imageService: 'compile',
  }),
});
