# Build fix: `builder-config.yaml: no such file or directory`

The collector Dockerfile runs:

```bash
/go/bin/builder --skip-strict-versioning --config=builder-config.yaml
```

That file must be present at the Docker build context root.

This bundle now includes a POC `builder-config.yaml` at the same level as `docker-compose.yml`.

Use:

```bash
cd flow2span-site-poc-bundle
docker compose build --no-cache collector
```

If you copy only the `docker/` folder into another repo, also copy:

```text
builder-config.yaml
connector/flow2spanconnector/
config/
```

The Dockerfile now fails with a clearer message if either `builder-config.yaml` or `connector/flow2spanconnector` is missing.
