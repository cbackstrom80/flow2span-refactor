# Fix: generator container `sudo: command not found`

The NetFlow generator simulates duplicate exporters by binding UDP sockets to fake exporter IPs such as `10.1.0.5` and `10.1.0.1`.

Inside Docker the container already runs as root, but the original helper script called `sudo ip addr add ...`. The slim Python image does not include `sudo`, so the loopback aliases were not created and the generator failed with:

```text
sudo: command not found
OSError: [Errno 99] Cannot assign requested address
```

This bundle fixes that by:

1. Running `ip addr add` directly when UID is `0`.
2. Falling back to `sudo` only when running on a non-root Linux/macOS host.
3. Ensuring the generator image includes `iproute2`.
4. Keeping `cap_add: [NET_ADMIN]` in `docker-compose.yml`.

Rebuild with:

```bash
docker compose build --no-cache generator
```

Then run:

```bash
docker compose --profile test up --build
```

Expected successful generator startup includes:

```text
adding loopback alias: 10.1.0.5
adding loopback alias: 10.1.0.1
adding loopback alias: 10.2.0.5
adding loopback alias: 10.2.0.1
adding loopback alias: 10.3.0.1
```
