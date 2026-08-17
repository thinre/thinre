import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

// One sidebar, ordered for a first-time reader: what it is, get it
// running, then the two contracts you will keep coming back to.
const sidebars: SidebarsConfig = {
  docs: [
    'index',
    'quickstart',
    {
      type: 'category',
      label: 'Reference',
      collapsed: false,
      items: ['integration-manifest', 'supervisor-configuration', 'link-protocol', 'cli'],
    },
  ],
};

export default sidebars;
