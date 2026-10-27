CREATE TABLE `tasks` (
	`id` text PRIMARY KEY NOT NULL,
	`plan_id` text NOT NULL,
	`key` text NOT NULL,
	`branch` text NOT NULL,
	`worktree_path` text,
	`base_ref` text NOT NULL,
	`created_at` integer NOT NULL,
	FOREIGN KEY (`plan_id`) REFERENCES `plans`(`id`) ON UPDATE no action ON DELETE cascade
);
--> statement-breakpoint
CREATE UNIQUE INDEX `tasks_plan_key_idx` ON `tasks` (`plan_id`,`key`);--> statement-breakpoint
ALTER TABLE `projects` ADD `allowed_commands` text DEFAULT '[]' NOT NULL;--> statement-breakpoint
ALTER TABLE `runs` ADD `task_id` text REFERENCES tasks(id) ON DELETE cascade;--> statement-breakpoint
ALTER TABLE `runs` ADD `before_sha` text;--> statement-breakpoint
ALTER TABLE `runs` ADD `after_sha` text;--> statement-breakpoint
CREATE INDEX `runs_task_idx` ON `runs` (`task_id`,`started_at`);