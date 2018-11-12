# chaind

chaind is a security and caching layer for blockchain nodes. Put chaind in front of geth, and be your own Infura!

Out-of-the box, chaind supports:

- Automatic failover to any blockchain node with an open RPC endpoint
- Intelligent request caching that takes chain reorgs into account
- RPC-aware request logging

## Architecture

chaind acts as a reverse proxy to one or more blockchain nodes. When it starts, it chooses one of those nodes to be the 'master' to which it will route RPC requests. In the background, it periodically healthchecks the master and automatically fails over to a replica if the healthcheck fails.

chaind attempts to serve all RPC requests from cache first. By default, chaind caches entire RPC response bodies in order to offload as much processing as possible to chaind and away from the master node.

> ⚠️ Currently, only Ethereum nodes are supported, however Bitcoin support will be added in the near future.

## Deployment

chaind compiles to a single binary that reads a config file, so deployment is a snap. Simply compile it, copy the example config file, and run it - that's it. There's an example supervisord config in the `build` folder as well should you wish to daemonize your chaind instance.

While chaind works without any kind of web server in front of it, for optimal performance we recommend proxying to chaind from a web server such as nginx. The web server can take care of gzipping responses, SSL termination, rate limiting, and a host of other features that you'll need in production better than chaind can.

## Roadmap

If people find chaind useful, we'll add the following new features in under three months:

1. Support for Bitcoin.
2. The ability to programmatically enable and disable particular RPC endpoints.
3. A web-based management UI.

## Why?

The stability of the Ethereum ecosystem is highly dependent on centralized infrastructure providers such as Infura. When Infura goes down, so does the chain. Single points of failure like this are rarely a positive thing. chaind aims to reduce that dependence, and make it possible for blockchain developers to leverage their own infrastructure with confidence.