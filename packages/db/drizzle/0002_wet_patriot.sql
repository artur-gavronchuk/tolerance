CREATE TABLE `plan_variants` (
	`id` text PRIMARY KEY NOT NULL,
	`plan_id` text NOT NULL,
	`position` integer NOT NULL,
	`json` text NOT NULL,
	`created_at` integer NOT NULL,
	FOREIGN KEY (`plan_id`) REFERENCES `plans`(`id`) ON UPDATE no action ON DELETE cascade
);
--> statement-breakpoint
CREATE INDEX `plan_variants_plan_idx` ON `plan_variants` (`plan_id`,`position`);--> statement-breakpoint
CREATE TABLE `plans` (
	`id` text PRIMARY KEY NOT NULL,
	`project_id` text NOT NULL,
	`request_id` text NOT NULL,
	`run_id` text NOT NULL,
	`status` text NOT NULL,
	`chosen_variant_id` text,
	`base_sha` text,
	`integration_branch` text,
	`created_at` integer NOT NULL,
	`approved_at` integer,
	FOREIGN KEY (`project_id`) REFERENCES `projects`(`id`) ON UPDATE no action ON DELETE cascade,
	FOREIGN KEY (`request_id`) REFERENCES `change_requests`(`id`) ON UPDATE no action ON DELETE cascade,
	FOREIGN KEY (`run_id`) REFERENCES `runs`(`id`) ON UPDATE no action ON DELETE cascade
);
--> statement-breakpoint
CREATE INDEX `plans_request_idx` ON `plans` (`request_id`,`created_at`);