# Worklane

Worklane provides tenant-scoped OTP messaging and is extending its domain with
short links and click analytics.

## Language

**Short link**:
A public URL that identifies a tenant-owned mapping to a destination URL.
_Avoid_: Private link, secret link

**Click**:
A recorded visit to a short link's redirect endpoint; repeated visits, bots and
previews may contribute, and the count does not represent unique people.
_Avoid_: Unique visitor, verified human click

**Destination URL**:
The immutable URL to which an active short link redirects visitors.
_Avoid_: Editable target

**Disabled link**:
A short link withdrawn from public redirection while its mapping and history
remain available to its owning tenant. Disabling is permanent within the MVP.
_Avoid_: Deleted link, expired link
