import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

import cloudflare from '@astrojs/cloudflare';

export default defineConfig({
  site: 'https://docs.herald-mail.app',

  integrations: [
    starlight({
      title: 'Herald Docs',
      description: 'User and integration documentation for Herald, the terminal email client for power users.',
      favicon: '/favicon.ico',
      head: [
        {
          tag: 'link',
          attrs: {
            rel: 'icon',
            type: 'image/png',
            sizes: '16x16',
            href: '/favicon-16x16.png',
          },
        },
        {
          tag: 'link',
          attrs: {
            rel: 'icon',
            type: 'image/png',
            sizes: '32x32',
            href: '/favicon-32x32.png',
          },
        },
        {
          tag: 'link',
          attrs: {
            rel: 'apple-touch-icon',
            sizes: '180x180',
            href: '/apple-touch-icon.png',
          },
        },
      ],
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
            { label: 'First-run wizard', slug: 'first-run-wizard' },
            { label: 'Run without email', slug: 'demo-mode' },
            { label: 'Provider setup', slug: 'provider-setup' },
            { label: 'Gmail IMAP', slug: 'gmail-setup' },
            { label: 'Custom IMAP', slug: 'custom-imap' },
          ],
        },
        {
          label: 'Using Herald',
          items: [
            { label: 'Global UI', slug: 'using-herald/global-ui' },
            { label: 'Timeline', slug: 'using-herald/timeline' },
            { label: 'Compose', slug: 'using-herald/compose' },
            { label: 'Cleanup', slug: 'using-herald/cleanup' },
            { label: 'Contacts', slug: 'using-herald/contacts' },
          ],
        },
        {
          label: 'Feature Guides',
          items: [
            { label: 'Search', slug: 'features/search' },
            { label: 'AI Features', slug: 'features/ai' },
            { label: 'Chat Panel', slug: 'features/chat' },
            { label: 'Rules and Automation', slug: 'features/rules-automation' },
            { label: 'Attachments', slug: 'features/attachments' },
            { label: 'Text Selection', slug: 'features/text-selection' },
            { label: 'Settings', slug: 'features/settings' },
            { label: 'Sync and Status', slug: 'features/sync-status' },
            { label: 'Destructive Actions', slug: 'features/destructive-actions' },
          ],
        },
        {
          label: 'Advanced',
          items: [
            { label: 'MCP Server', slug: 'advanced/mcp' },
            { label: 'SSH Mode', slug: 'advanced/ssh-mode' },
            { label: 'Daemon Commands', slug: 'advanced/daemon' },
            { label: 'Demo GIF Workflow', slug: 'advanced/demo-gifs' },
            { label: 'Privacy and Security', slug: 'security-privacy' },
          ],
        },
        {
          label: 'Reference',
          items: [
            { label: 'All Keybindings', slug: 'reference/keybindings' },
            { label: 'Config Reference', slug: 'reference/config' },
            { label: 'Troubleshooting', slug: 'troubleshooting' },
            { label: 'Uninstall', slug: 'uninstall' },
          ],
        },
      ],
    }),
  ],

  adapter: cloudflare({
    imageService: 'compile',
  }),
});
