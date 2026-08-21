# Try TypeRB with Docker

Run the example with Docker Compose:

```sh
docker compose run --rm typerb
```

```text
Hello from TypeRB!
```

Open a shell in the same environment to run the compiler or REPL directly:

```sh
docker compose run --rm typerb bash
trb run hello.trb
trb repl
```

The TypeRB and Go versions can be selected when rebuilding the image:

```sh
docker compose build \
  --build-arg TYPERB_VERSION=X.Y.Z \
  --build-arg GO_VERSION=1.27
```
