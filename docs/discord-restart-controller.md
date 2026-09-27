# Discord administrator restart controller

## Commands

- `/restart`: restart only the `automuteus` Compose service.
- `/restart-all`: stop AutoMuteUs and Galactus, restart Redis/PostgreSQL, wait for both healthchecks, then start Galactus and AutoMuteUs.

Both commands use Discord's real `Administrator` permission as the default command permission and re-check `PermissionAdministrator` at execution time.

## Compose additions

Add these environment variables to `automuteus.environment`:

```yaml
RESTART_CONTROLLER_URL: "http://restart-controller:8090"
RESTART_CONTROLLER_TOKEN: "${RESTART_CONTROLLER_TOKEN:?err}"
```

Add this service:

```yaml
  restart-controller:
    build:
      context: ./restart-controller
    restart: always
    environment:
      TZ: Asia/Tokyo
      RESTART_CONTROLLER_TOKEN: "${RESTART_CONTROLLER_TOKEN:?err}"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
```

The controller determines the current Compose project from its own Docker labels and only looks up the fixed service names `automuteus`, `galactus`, `redis`, and `postgres`.

Generate a long random value for `RESTART_CONTROLLER_TOKEN` and keep it only in the deployment `.env`.

## Safety properties

- AutoMuteUs itself does not receive the Docker socket.
- The HTTP endpoint accepts only `bot` or `all`; arbitrary Docker commands or container names cannot be supplied.
- Only one restart operation can run at a time.
- Redis and PostgreSQL named volumes are never removed.
- `/restart-all` waits for the existing Redis and PostgreSQL Docker healthchecks before restarting dependents.
- Completion/failure is posted using the originating Discord interaction webhook token, so the controller does not need the Discord bot token.
