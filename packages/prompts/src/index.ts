/// <reference path="./raw.d.ts" />
import assessTemplate from './assess.md?raw';
import mapTemplate from './map.md?raw';
import validationTemplate from './validation.md?raw';

/** Fills `{{name}}` placeholders; a placeholder without a value is a bug, so it throws. */
export function render(template: string, vars: Record<string, string>): string {
  return template.replace(/\{\{(\w+)\}\}/g, (_, name: string) => {
    const value = vars[name];
    if (value === undefined) throw new Error(`Prompt placeholder {{${name}}} has no value`);
    return value;
  });
}

export function mapPrompt(vars: { scan: string; tree: string; language: string }): string {
  return render(mapTemplate, vars);
}

export function assessPrompt(vars: { task: string; context: string; language: string }): string {
  return render(assessTemplate, vars);
}

/** Follow-up when the agent's answer does not pass validation (SPEC §3.2). */
export function validationFollowUp(problems: string[]): string {
  return render(validationTemplate, { problems: problems.map((p) => `- ${p}`).join('\n') });
}
