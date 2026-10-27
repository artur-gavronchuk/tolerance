import type { Messages } from './ru';

export const en: Messages = {
  sidebar: {
    projects: 'Projects',
    addProject: 'Add project',
    noProjects: 'No projects yet',
  },
  welcome: {
    title: 'Open a repository to plan a change',
    lead: 'Studio maps your project, proposes plan variants, runs agents in separate worktrees and checks the result against the approved plan.',
    addProject: 'Add project',
    addProjectSoon: 'Adding projects comes in the next step.',
    steps: {
      map: { title: 'Map', text: 'An agent describes the components and how they connect' },
      plan: { title: 'Plan', text: 'Two variants with tasks, dependencies and acceptance criteria' },
      run: { title: 'Run', text: 'Independent tasks run in parallel, each in its own worktree' },
      verify: { title: 'Verify', text: 'Tests, scope control and a verdict for every criterion' },
      review: { title: 'Review', text: 'One diff on a studio/* branch, comments go back to the agent' },
    },
  },
  settings: {
    language: 'Language',
    theme: 'Theme',
    themeSystem: 'System',
    themeLight: 'Light',
    themeDark: 'Dark',
  },
  status: {
    runtime: 'Electron {{electron}}, Node {{node}}',
  },
};
