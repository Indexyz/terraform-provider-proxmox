# Examples

This directory contains examples that are used for documentation and manual testing.

The documentation tool looks for:

* **provider/provider.tf** for the provider index page
* **data-sources/`full data source name`/data-source.tf** for a data source page
* **resources/`full resource name`/resource.tf** for a resource page

Keep QEMU VM examples aligned with the generated schema. When advanced clone, disk, network, and cloud-init domains change, update the example files and regenerate docs in the same change so the published snippets do not drift from the provider surface.
