# Plank

## What is Plank?
Plank is just enough of a platform to build whatever you want on top. It is a small yet powerful Golang server that can serve
static contents, single page applications, create and expose microservices over REST endpoints or WebSocket via a built-in
STOMP broker, or even interact directly with other message brokers such as RabbitMQ. All this is done in a consistent
and easy to follow manner powered by Transport Event Bus.

Writing a service for a Plank server is in a way similar to writing a Spring Boot `Component` or `Service`, because a lot of tedious
plumbing work is already done for you such as creating an instance of a service and wiring it up with HTTP endpoints using routers etc.
Just by following the API you can easily stand up a service, apply any kinds of middleware your application logic calls for, and do all these
dynamically while in runtime, meaning you can conditionally apply a filter for certain REST endpoints, stand up a new service on demand, or
even spawn yet another whole new instance of Plank at a different endpoint.

All features are cleanly exposed as public API and modules and, combined with the power of Golang's concurrency model using channels,
the Transport Event Bus allows creating a clean application architecture, with straightforward and easy to follow logic.

Detailed tutorials and examples are currently in progress and will be made public on the [Transport GitHub page](https://vmware.github.io/transport/).
Some topics that will initially be covered are:

- Writing a simple service and interacting with it over REST and WebSocket
- Service lifecycle hooks
- Middleware management (for REST bridged services)
- Concept of local bus channel and galactic bus channel
- Communicating between Plank instances using the built-in STOMP broker
- Securing your REST and WebSocket endpoints using Auth Provider Manager

## Hello world
### How to build Plank
First things first, you'll need Golang SDK of at least version 1.16 because it uses a built-in
package that was added as of 1.16. Once you made sure your Golang version is at least 1.16,
follow the commands below:

```bash
# get all dependencies
go get ./...

# To build against your operating system
go run build.go

# or, to build against a specific operating system
BUILD_OS=darwin|linux|windows go run build.go
```

Once successfully built, `plank` binary will be ready under `build/`.

### Generate a self signed certificate
Plank can run in non-HTTPS mode but it's generally a good idea to always do development in a similar environment where you'll be serving your
audience in public internet (or even intranet). Plank repository comes with a handy utility script that can generate a pair of server certificate
and its matching private key at `scripts/create-selfsigned-cert.sh`. Simply run it in a POSIX shell like below. (requires `openssl` library
to be available):

```bash
# generate a pair of x509 certificate and private key files
./scripts/generate-selfsigned-cert.sh

# cert/fullchain.pem is your certificate and cert/server.key its matching private key
ls -la cert/
```

### The real Hello World part
Now we're ready to start the application. To kick off the server using the demo app you have built above, type the following and you'll see something like this:

```bash
./build/plank start-server --config-file config.json

 P L A N K
Host                    localhost
Port                    30080
Fabric endpoint         /ws
SPA endpoint            /
SPA static assets       /assets
Health endpoint         /health
Prometheus endpoint     /prometheus
...
time="2021-08-17T13:28:15-07:00" level=info msg="Service '*services.StockTickerService' initialized successfully" fileName=initialize.go goroutine=44 package=server
time="2021-08-17T13:28:15-07:00" level=info msg="Service channel 'stock-ticker-service' is now bridged to a REST endpoint /rest/stock-ticker/{symbol} (GET)\n" fileName=server.go goroutine=44 package=server
time="2021-08-17T13:28:15-07:00" level=info msg="Starting Fabric broker at localhost:30080/ws" fileName=server.go goroutine=1 package=server
time="2021-08-17T13:28:15-07:00" level=info msg="Starting HTTP server at localhost:30080 with TLS" fileName=server.go goroutine=3 package=server
```

Now, open your browser and navigate to https://localhost:30080/rest/stock-ticker/VMW (or
type `curl -k https://localhost:30080/rest/stock-ticker/VMW` in Terminal if you prefer CLI),
and accept the self-signed certificate warning. You will be served a page that shows the latest stock price
for VMware, Inc. Try and swap out `VMW` with another symbol of your choice to further test it out!

If you navigate to the root at https://localhost:30080, you'll be greeted with a 404!
This is an expected behavior, as the demo app does not serve anything at root `/`, but we will
consider changing the default 404 screen to something that is informational or more appealing at least.

> NOTE: The sample service is using a loosely gated third party API which imposes
> a substantial limit on how many calls you can make per minute and per day in return for making
> the service free to all.

> NOTE: Plank, when started, can boot up with a nice image of a wooden plank
> as a splash screen, but it's disabled by default because of a few reasons like unnecessarily
> adding to the binary size and generally being a bloat to some consumers. If you want
> to see it though, build your app with `--tags boot_img` custom tag.

## All supported flags and usages

|Long flag|Short flag|Default value|Required|Description|
|----|----|----|----|----|
|--hostname|-n|localhost|false|Hostname where Plank is to be served. Also reads from `$PLANK_SERVER_HOSTNAME` environment variable|
|--port|-p|30080|false|Port where Plank is to be served. Also reads from `$PLANK_SERVER_PORT` environment variable|
|--rootdir|-r|<current directory>|false|Root directory for the server. Also reads from `$PLANK_SERVER_ROOTDIR` environment variable|
|--static|-s|-|false|Path to a location where static files will be served. Can be used multiple times|
|--no-fabric-broker|-|false|false|Do not start Fabric broker|
|--fabric-endpoint|-|/fabric|false|Fabric broker endpoint (ignored if --no-fabric-broker is present)|
|--topic-prefix|-|/topic|false|Topic prefix for Fabric broker (ignored if --no-fabric-broker is present)|
|--queue-prefix|-|/queue|false|Queue prefix for Fabric broker (ignored if --no-fabric-broker is present)|
|--request-prefix|-|/pub|false|Application request prefix for Fabric broker (ignored if --no-fabric-broker is present)|
|--request-queue-prefix|-|/pub/queue|false|Application request queue prefix for Fabric broker (ignored if --no-fabric-broker is present)|
|--shutdown-timeout|-|5|false|Graceful server shutdown timeout in minutes|
|--output-log|-l|stdout|false|File to output platform logs to|
|--access-log|-l|stdout|false|File to output HTTP server access logs to|
|--error-log|-l|stderr|false|File to output HTTP server error logs to|
|--debug|-d|false|false|Debug mode|
|--no-banner|-b|false|false|Do not print Plank banner at startup|
|--prometheus|-|false|false|Enable Prometheus at /prometheus for metrics|
|--rest-bridge-timeout|-|1|false|Time in minutes before a REST endpoint for a service request to timeout|

Examples are as follows:
```bash
# Start a server with all options set to default values
./plank start-server

# Start a server with a custom hostname and at port 8080 without Fabric (WebSocket) broker
./plank start-server --no-fabric-broker --hostname my-app.io --port 8080

# Start a server with a 10 minute graceful shutdown timeout
# NOTE: this is useful when you run a service that takes a significant amount of time or might even hang while shutting down 
./plank start-server --shutdown-timeout 10

# Start a server with platform server logs printing to stdout and access/error logs to their respective files
./plank start-server --access-log server-access-$(date +%m%d%y).log --error-log server-error-$(date +%m%d%y).log

# Start a server with debug outputs enabled
./plank start-server -d

# Start a server without splash banner
./plank start-server --no-banner

# Start a server with Prometheus enabled at /prometheus
./plank start-server --prometheus

# Start a server with static path served at `/static` for folder `static`
./plank start-server --static static 

# Start a server with static paths served at `/static` for folder `static` and
# at `/public` for folder `public-contents`
./plank start-server --static static --static public-contents:/public

# Start a server with a JSON configuration file
./plank start-server --config-file config.json
```

## Advanced topics (WIP. Coming soon)
### OAuth2 Client
Plank supports seamless out of the box OAuth 2.0 client that support a few OAuth flows. such as authorization
code grant for web applications and client credentials grant for server-to-server applications.
See below for a detailed guide for each flow.

#### Authorization Code flow
You'll choose this authentication flow when the Plank server acts as an intermediary that exchanges
the authorization code returned from the authorization server for an access token. During this process you will be
redirected to the identity provider's page like Google where you are asked to confirm the type of 3rd party application and
its scope of actions it will perform on your behalf and will be taken back to the application after successful authorization.

#### Client Credentials flow
You'll choose this authentication flow when the Plank server needs to directly communicate with another backend service.
This will not require user's consent like you would be redirected to Google's page where you confirm the type of application
requesting your consent for the scope of actions it will perform on your behalf. not requiring any interactions from the user. You will need to
create an OAuth 2.0 Client with `client_credentials` grant before following the steps below to implement the
authentication flow.
