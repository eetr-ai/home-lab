-- The declared Helm deployments, and the values written for each of them.
--
-- This is desired state and nothing else. What is actually running — a release's
-- status, its revision, its rendered notes — is read live from Helm's own storage
-- on every request, because the cluster is the only thing that knows. Two records
-- of one truth would disagree the first time a pod was killed mid-upgrade.

CREATE TABLE helm_deployments (
    id           uuid PRIMARY KEY,
    -- The namespace and release name together identify the release on the
    -- cluster, so they are unique together here: two records pointing at one
    -- release would each believe they owned its values.
    namespace    text        NOT NULL,
    release_name text        NOT NULL,
    -- The reference as the operator typed it: oci://host/path/chart, or an https
    -- chart repository. Never carries credentials — ParseChartRef refuses them
    -- before anything reaches this table.
    chart_ref    text        NOT NULL,
    created_by   text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (namespace, release_name)
);

-- One row per (chart version, values) pair that has ever been declared.
--
-- Append-only. Editing values in the panel writes a new version; a pipeline
-- posting overrides writes a new version. Nothing is updated in place except the
-- rollout stamp, so "what did the pipeline change, and when" is a query rather
-- than an archaeology exercise.
CREATE TABLE helm_deployment_versions (
    deployment_id uuid        NOT NULL REFERENCES helm_deployments (id) ON DELETE CASCADE,
    version       integer     NOT NULL,
    chart_version text        NOT NULL,
    -- YAML rather than jsonb, and one column rather than two. A values file is a
    -- document somebody wrote: round-tripping it through JSON would strip its
    -- comments and reorder its keys, and the whole point of this feature is that
    -- an operator opens the editor and sees their own file. Nothing here queries
    -- inside the values, which is the only thing jsonb would have bought.
    values_yaml   text        NOT NULL,
    -- 'panel' or 'ci'. Which one wrote a version is the first thing anybody asks
    -- when a release changed and nobody remembers doing it.
    source        text        NOT NULL,
    created_by    text        NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    -- Null until this version reached the cluster. A newest version with a null
    -- stamp is what the panel calls "not rolled out".
    rolled_out_at timestamptz,
    -- The Helm revision this version produced, so a version can be lined up
    -- against the release history Helm keeps.
    helm_revision integer,
    PRIMARY KEY (deployment_id, version),
    CONSTRAINT helm_deployment_versions_source CHECK (source IN ('panel', 'ci'))
);

CREATE INDEX helm_deployments_namespace ON helm_deployments (namespace);
