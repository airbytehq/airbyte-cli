package browserlogin

import "time"

// DefaultKeycloakBase is the Keycloak realm authority that issues access
// tokens for production app.airbyte.ai users. Confirmed against sonar's
// frontend/.env.production (KEYCLOAK_URL + realm "_airbyte-cloud-users") and
// frontend/src/core/services/auth/KeycloakAuthProvider.tsx.
const DefaultKeycloakBase = "https://cloud.airbyte.com/auth/realms/_airbyte-cloud-users"

// DefaultClientID is the public Keycloak client id used by both the sonar
// web app and this CLI. Public client (no secret) — PKCE is required.
const DefaultClientID = "sonar-webapp"

// CallbackPath is the path the loopback server listens on for the OAuth
// redirect_uri. The full redirect_uri is http://127.0.0.1:<ephemeral>/callback.
const CallbackPath = "/callback"

// DefaultScopes are the OAuth/OIDC scopes requested at the authorize step.
// "offline_access" yields a refresh_token so future phases can renew silently.
const DefaultScopes = "openid email profile offline_access"

// DefaultTimeout caps the end-to-end browser flow (open → callback → token
// exchange). Three minutes mirrors common CLI OAuth conventions and is enough
// time to complete an IdP redirect chain (Google, SSO, etc.).
const DefaultTimeout = 3 * time.Minute
