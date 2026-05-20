# Architecture

## Architectural Style

Adopt ports&adapters (Hexagonal) architectural style:

- A port is an interface that acts as an entry and/or exit point, without consumers knowing the concrete implementation.
- An adapter is a class that transforms a request to an application operation call, or adapts an external interface **in a broad sense** into an internal interface.
  - two kinds of adapters: driving adapters, driven adapters
  - driving adapters: accept external requests, call application operations (i.e. some of the Ports) to perform use cases.
  - driven adapters
    - always react to an action spawned by a driving adapter
    - implement a Port, an interface, and adapts external APIs to the Port.
- For this architecture to work as it should, it is of utmost importance that the Ports are created to fit the Application Core needs and not simply mimic the external service or infrastructure APIs.

### Architecture Instructions

- three vertical layers: Interfaces, Application and Domain, each is supported by different driven adapters. Any element of a layer depends only on other elements in the same layer or on elements of the layers **beneath** it.
- The driven-adapters layer depends on the infrastructure layer, and only it can depend on the infrastructure.
- Interfaces
  - holds everything that interacts with other systems, such as web applications, other services, etc.
  - handles
    - interpretation, validation and translation of incoming data
    - serialization of outgoing data
  - are essentially driving adapters.
- Application
  - defines and implements the use cases. Further to that, directly implement application logic but delegate to encapsulated domain objects for domain logic.
  - consists of
    - application Services with their interfaces and/or Command Handlers, which contain the logic to perform a use case, a business process.
    - application-logic-oriented service interfaces such as search engine interface, messaging interface, query interface, push notification API, and so on.
    - transaction control, security check and so on
- Domain
  - carries the specialized knowledge (i.e. domain logic) of a problem domain, which is accumulated over time, and changes relatively infrequently.
  - consists of
    - domain service: carries the domain logic that involves different entities, of the same type or not, and does not belong in the entities themselves.
    - domain entities, value objects, domain events, aggregates, repository interfaces for entities or aggregates and/or events.
- Driven Adapters
  - adapts external services APIs to internal APIs that meets the Application Core needs.
- Infrastructure
  - provides generic technical capabilities that can be reused by different driven adapters.
  - optional, on demand. For example, an external service is used in multiple places, and doesn't provide a client, or the APIs are difficult to use, this is the case where it is necessary to define an infrastructure component.
