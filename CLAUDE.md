Testing
=======

Run the following commands
```
devpod delete 'vscode-remote-try-node'
devpod provider delete nomad
RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev

devpod up github.com/microsoft/vscode-remote-try-node --provider nomad --debug
```
