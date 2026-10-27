export const ru = {
  sidebar: {
    projects: 'Проекты',
    addProject: 'Добавить проект',
    noProjects: 'Проектов пока нет',
  },
  welcome: {
    title: 'Откройте репозиторий, чтобы спланировать изменение',
    lead: 'Studio строит карту проекта, предлагает варианты плана, запускает агентов в отдельных worktree и проверяет результат относительно утверждённого плана.',
    addProject: 'Добавить проект',
    addProjectSoon: 'Добавление проектов появится на следующем шаге.',
    steps: {
      map: { title: 'Карта', text: 'Агент описывает компоненты проекта и связи между ними' },
      plan: { title: 'План', text: 'Два варианта с задачами, зависимостями и критериями приёмки' },
      run: { title: 'Исполнение', text: 'Независимые задачи идут параллельно, каждая в своём worktree' },
      verify: { title: 'Проверка', text: 'Тесты, контроль scope и вердикт по каждому критерию' },
      review: { title: 'Ревью', text: 'Общий diff в ветке studio/*, комментарии уходят агенту' },
    },
  },
  settings: {
    language: 'Язык',
    theme: 'Тема',
    themeSystem: 'Система',
    themeLight: 'Светлая',
    themeDark: 'Тёмная',
  },
  status: {
    runtime: 'Electron {{electron}}, Node {{node}}',
  },
};

export type Messages = typeof ru;
