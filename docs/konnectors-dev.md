[Table of contents](README.md#table-of-contents)


# running a konnector locally



## Locally but linked to a Cozy

This method requires a working cozy-stack. It will allow you to run the konnector
outside the cozy-stack but saves the data inside the cozy-stack. This can be useful
when your konnector is not available in one of the registries but you still want to save
you data in your real Cozy.

You can find a detailed explanation inside the [linking your konnector to a cozy](https://docs.cozy.io/en/tutorials/konnector/save-data/#linking-your-connector-to-a-cozy-dev-mode)
page.



## Installed from a local directory

This method requires a working cozy-stack with the admin permissions and the cozy-stack 
CLI. It will allow you to install your konnector exactly like it was published in a registry. This 
can be a good way to check locally if everything works well before publishing 
to the registry.


### Installing the konnector

First build it:

```
yarn build
```

Start your cozy-stack server with the `--dev` flag. If you have the cozy-stack source code
locally you can run this following command for example:
```
go run . serve --dev --config ~/.config/cozy/config.yaml --mailhog --fs-url=file://localhost${PWD}/storage --konnectors-cmd ${PWD}/scripts/konnector-dev-run.sh
```

> You will need to have a valid [configuration file](./config.md).


Then you just have to run:

```
cozy-stack konnectors install <slug> files://<konnector folder absolute path>/build
```

> Be careful to point the `/build` folder inside your konnector folder!


### Run the konnector

Once connected to your Cozy instance (probably `http://cozy.localhost:8080`) you should 
have a new icon with your konnector name. Once you click on it, you will be asked the informations 
required to run the konnector. Once saved, the konnector will start running.


> After modifying the konnector, clicking on the `Synchronize` button will fetch
the new code an run it with the changes.
