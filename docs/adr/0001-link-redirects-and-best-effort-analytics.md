# Prioritize redirects over complete click accounting

Link Service keeps public redirection independent of Kafka delivery, using
bounded asynchronous emission and a separate analytics consumer. The owner
accepted possible event loss on overflow/outage and duplicate counting on
redelivery instead of making redirects wait for durable analytics delivery.
Click totals therefore represent approximate recorded redirect activity rather
than unique-human or reconciliation-grade counts; durable, exactly-once
accounting would require a separate design decision.

The [MVP scope](../roadmap/2026-11-15-link-service-mvp-scope.md) records confirmed
semantics and acceptance criteria. The owner confirmed the consolidated scope
on 2026-11-15 before ticket breakdown.
