import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

// Public documentation for Thinre, served at https://thinre.github.io/thinre/
// (GitHub Pages, deployed by .github/workflows/docs.yml on every merge to
// main). Docs-only mode: the docs ARE the site, mounted at the root.

const config: Config = {
  title: 'Thinre',
  tagline: 'Universal lifecycle control plane for black-box software',
  favicon: 'img/favicon.ico',

  future: {
    v4: true,
  },

  url: 'https://thinre.github.io',
  baseUrl: '/thinre/',

  organizationName: 'thinre',
  projectName: 'thinre',
  trailingSlash: false,

  onBrokenLinks: 'throw',

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          routeBasePath: '/', // docs-only site
          sidebarPath: './sidebars.ts',
          editUrl: 'https://github.com/thinre/thinre/tree/main/website/',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    navbar: {
      title: 'Thinre',
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docs',
          position: 'left',
          label: 'Documentation',
        },
        {
          href: 'https://github.com/thinre/thinre',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [],
      copyright: `Copyright © ${new Date().getFullYear()} Thinre. Apache-2.0 licensed.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['bash', 'yaml'],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
