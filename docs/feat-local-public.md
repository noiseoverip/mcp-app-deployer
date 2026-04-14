This feature is to add an option to either deploy it publicly or locally.
Locally - app is accessible only on the local network.
Publicly - app is exposed to the public internet.

# Public
Public application are deployed to namespace "applications" by default and not network policy is applied.

# Local
Local applications are designed to be access only from a local network.
To achieve this, they are deployed to a separate namespace (defaults to "applications-local"), and a network policy is
applied on that namespace which restricts subnets from which traffic is allowed. Subnets are configurable at startup.