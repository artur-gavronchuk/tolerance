CREATE TABLE `workspaces` (
	`id` text PRIMARY KEY NOT NULL,
	`project_id` text NOT NULL,
	`name` text NOT NULL,
	`slug` text NOT NULL,
	`branch` text NOT NULL,
	`path` text NOT NULL,
	`base_ref` text NOT NULL,
	`base_sha` text NOT NULL,
	`setup_status` text NOT NULL,
	`setup_exit_code` integer,
	`setup_log` text DEFAULT '' NOT NULL,
	`created_at` integer NOT NULL,
	FOREIGN KEY (`project_id`) REFERENCES `projects`(`id`) ON UPDATE no action ON DELETE cascade
);
--> statement-breakpoint
CREATE UNIQUE INDEX `workspaces_project_slug_idx` ON `workspaces` (`project_id`,`slug`);--> statement-breakpoint
ALTER TABLE `projects` ADD `editor_command` text;--> statement-breakpoint
ALTER TABLE `projects` ADD `terminal_app` text;