## landscaper-cli installations

commands to interact with installations

### Options

```
  -h, --help   help for installations
```

### Options inherited from parent commands

```
      --cli                  logger runs as cli logger. enables cli logging
      --dev                  enable development logging which result in console encoding, enabled stacktrace and enabled caller
      --disable-caller       disable the caller of logs (default true)
      --disable-stacktrace   disable the stacktrace of error logs (default true)
      --disable-timestamp    disable timestamp output (default true)
  -v, --verbosity int        number for the log level verbosity (default 1)
```

### SEE ALSO

* [landscaper-cli](landscaper-cli.md)	 - landscaper cli
* [landscaper-cli installations create](landscaper-cli_installations_create.md)	 - create an installation template for a component which is stored in an OCI registry
* [landscaper-cli installations force-delete](landscaper-cli_installations_force-delete.md)	 - Deletes an installations and the depending executions and deployItems in cluster and namespace of the current kubectl cluster context. Concerning the deployed software no guarantees could be given if it is uninstalled or not.
* [landscaper-cli installations inspect](landscaper-cli_installations_inspect.md)	 - Displays status information for all installations and depending executions and deployItems in cluster and namespace of the current kubectl cluster context. To display only one installation, specify the installation-name.
* [landscaper-cli installations set-import-parameters](landscaper-cli_installations_set-import-parameters.md)	 - Set import parameters for an installation. Quote values containing spaces in double quotation marks.

