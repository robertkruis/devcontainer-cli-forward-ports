# devcontainer-cli-forward-ports

## Overview
This project enables support for the devcontainer `forwardPorts` configuration option when making direct use of the **devcontainer CLI**. As the **devcontainer CLI** currently [does not support port forwarding](https://github.com/devcontainers/cli/issues/22), the resulting binary of this project works around this issue.

This project is inspired by the original python version: https://github.com/nohzafk/devcontainer-cli-port-forwarder, so all credits for the forwarding implementation go to [@nohzafk](https://github.com/nohzafk). However, the fact that it requires python to be installed on the host machine, made me try to implement it in [Go](https://go.dev/). Other than that, it was a great excuse to get to learn Go.

> [!IMPORTANT]
> The project has only been tested on Windows, but should work on other systems as well.

## Usage context
Usage is only needed when using the **devcontainer CLI** directy. If using any other devcontainer management solution, this probably is already managed.

## Prerequisites
Make sure `socat` is installed into the `container`. See [Installation](#installation) for further details. Also the `docker` executable must be present on the path.

## Features
- Automatically forwards the forwarded ports from the `devcontainer.json` from the host to a container.
- When a container is shutdown, the application will shutdown itself.
- Instead of executing `docker inspect` on an interval to check whether the container is running, the `docker events` command is used to get notifications about starts and shutdowns of the container.

## Installation
Make sure the container has `socat` installed, as it is required for the forwarding to work correctly. You can use the `onCreateCommand` in the `devcontainer.json` to install it automatically:
```json
"onCreateCommand": "sudo apt update && sudo apt install -y socat",
```

## Building
Clone this project, make sure you have the latest version of Go installed, and build the binary using:
```
go build ./cmd/forwardports
```

## Usage

### Devcontainer configuration
Copy the binary to your `.devcontainer` directory (or any other location) and make sure it is executed in the background when the container is initialized. To enable this automatically, change the `initializeCommand` option in the `devcontainer.json` to the following:
```json
"initializeCommand": ".devcontainer/forwardports &"
```

> [!IMPORTANT]
> When running on Windows, the `initializeCommand` should be changed as invocation on the background does not work as expected. Use the below `initializeCommand` setting in your `devcontainer.json`. This will ensure the correct `workspace folder` is used when starting the forwarder.
> ```json
> "initializeCommand": [
>		"powershell",
>		"start-process",
>		"-filepath",
>		".devcontainer/forwardports",
>		"-argumentlist",
>		"'-workspace-folder ${localWorkspaceFolder}'",
>		"-windowstyle",
>		"Hidden"
>	],
> ```

### CLI
The `forwardports` CLI also accepts the following flags:
- `-h` show the CLI usage help
- `-debug` enables debug logging with regards to the forwarding and `docker events` monitoring
- `-workspace-folder` sets the workspace folder on which the forwarder should operate. The default value will be the current working directory from where the executable is started.

## Acknowledgements
- [@nohzafk](https://github.com/nohzafk) for the [devcontainer-cli-port-forwarder](https://github.com/nohzafk/devcontainer-cli-port-forwarder) project that was a great resource.
- The [go-cmd/cmd](https://github.com/go-cmd/cmd) package from which the code for streaming command output was taken.
