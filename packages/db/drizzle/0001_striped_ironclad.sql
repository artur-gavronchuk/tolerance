CREATE TABLE `assessments` (
	`id` text PRIMARY KEY NOT NULL,
	`request_id` text NOT NULL,
	`run_id` text NOT NULL,
	`json` text NOT NULL,
	`created_at` integer NOT NULL,
	FOREIGN KEY (`request_id`) REFERENCES `change_requests`(`id`) ON UPDATE no action ON DELETE cascade,
	FOREIGN KEY (`run_id`) REFERENCES `runs`(`id`) ON UPDATE no action ON DELETE cascade
);
--> statement-breakpoint
CREATE INDEX `assessments_request_idx` ON `assessments` (`request_id`,`created_at`);--> statement-breakpoint
CREATE TABLE `change_requests` (
	`id` text PRIMARY KEY NOT NULL,
	`project_id` text NOT NULL,
	`text` text NOT NULL,
	`created_at` integer NOT NULL,
	FOREIGN KEY (`project_id`) REFERENCES `projects`(`id`) ON UPDATE no action ON DELETE cascade
);
--> statement-breakpoint
CREATE INDEX `change_requests_project_idx` ON `change_requests` (`project_id`,`created_at`);--> statement-breakpoint
ALTER TABLE `runs` ADD `request_id` text REFERENCES change_requests(id) ON DELETE cascade;--> statement-breakpoint
CREATE INDEX `runs_request_idx` ON `runs` (`request_id`,`started_at`);