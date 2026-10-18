-- A money event can commit before detached physical work ends; those are separate facts.
ALTER TABLE cluster_executions ADD COLUMN IF NOT EXISTS work_finished BOOLEAN NOT NULL DEFAULT FALSE;
