# Detection pack

Rules for defenders watching for an ARTEX console or for the prebuilt images that were reported backdoored in November 2026. This pack does not detect a technique and it does not include a proof of concept. It matches process names, the console port, and image names.

These rules will fire on a lab that is actually running this distribution. That is the point of the process rule. The image rules should not fire on a source build.

| Path | Use |
| --- | --- |
| [sigma/artex_console_process.yml](sigma/artex_console_process.yml) | Linux process creation for a binary named `artex` |
| [sigma/artex_bad_image.yml](sigma/artex_bad_image.yml) | Command lines that name the reported images |
| [suricata/artex.rules](suricata/artex.rules) | HTTP bodies or URIs that name those images, and a separate console-port notice |
| [yara/artex_images.yar](yara/artex_images.yar) | The same image names inside a file |
| [indicators/images.csv](indicators/images.csv) | Machine-readable image list |

`python3 detections/tests/test_pack.py` checks that every file is present and that the image names are spelled the same way in each one. It does not need Sigma or Suricata installed.

The image names are identifiers:

* `tomicdd/artex`
* `congdonglinux/artex`

Do not pull them in order to test the rule.
