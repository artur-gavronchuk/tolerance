CREATE TABLE `terminals` (
	`id` text PRIMARY KEY NOT NULL,
	`workspace_id` text NOT NULL,
	`kind` text NOT NULL,
	`position` integer NOT NULL,
	`session_id` text,
	`created_at` integer NOT NULL,
	FOREIGN KEY (`workspace_id`) REFERENCES `workspaces`(`id`) ON UPDATE no action ON DELETE cascade
);
--> statement-breakpoint
CREATE INDEX `terminals_workspace_idx` ON `terminals` (`workspace_id`,`position`);--> statement-breakpoint
ALTER TABLE `projects` ADD `agent_command` text;