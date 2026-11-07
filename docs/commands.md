# Commands

| command | does |
|---|---|
| `vessel build <dir> -o myapp --key vessel.key` | pins, packs, signs |
| `./myapp install` | installs and starts the stack |
| `./myapp upgrade` | installs a newer version, removes what it no longer carries |
| `./myapp resume` | finishes a cut install |
| `./myapp status` | what is installed, what runs |
| `./myapp uninstall` | removes what the install wrote |
| `./myapp inspect` | what the file carries |

`--dry-run` on `install`, `upgrade` and `resume` prints the plan and changes nothing.

Exit codes: `0` done, `3` machine not ready, `4` refused, nothing touched, `5` failed partway.

## After a power cut

```sh
$ ./myapp status
this install never finished, it stopped after images: run resume, or install with the 1.4.0 installer
```

`resume` finishes it, the previous installer rolls back, `uninstall` removes both.
