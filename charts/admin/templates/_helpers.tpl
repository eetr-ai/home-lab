{{- define "home-lab-admin.labels" -}}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: home-lab-admin
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end }}

{{- /*
The tag every image carries.

One tag rather than one per image because one pipeline builds them all from one commit,
and it defaults to the chart's own appVersion because that same pipeline gives
the chart and the images one version -- admin-v1.4.0 publishes 1.4.0 everywhere.
So the normal case is a values file that names no tag at all, and installing
chart 1.5.0 installs the 1.5.0 images by construction rather than because
somebody remembered to change a second number.

release-please keeps Chart.yaml's appVersion in step with the release, so this
holds for a checkout as well as for a published chart.

There is deliberately no default of "latest", and it is refused here rather than
only in values.schema.json: the schema does not see a value that arrives through
appVersion, so a chart packaged with --app-version latest would otherwise sail
past it. A pod pulling a moving name cannot say which build it is running.
*/ -}}
{{- define "home-lab-admin.imageTag" -}}
{{- $tag := .Values.admin.image.tag | default .Chart.AppVersion -}}
{{- if not $tag -}}
{{- fail "admin.image.tag is empty and this chart has no appVersion to fall back on" -}}
{{- end -}}
{{- if eq $tag "latest" -}}
{{- fail "admin.image.tag must be an immutable tag, and \"latest\" is not one" -}}
{{- end -}}
{{- $tag -}}
{{- end }}

{{- /*
The API image.
*/ -}}
{{- define "home-lab-admin.apiImage" -}}
{{- printf "%s:%s" .Values.admin.api.image.repository (include "home-lab-admin.imageTag" .) -}}
{{- end }}

{{- /*
The panel image, addressed the same way and for the same reason as the API's.
*/ -}}
{{- define "home-lab-admin.webImage" -}}
{{- printf "%s:%s" .Values.admin.web.image.repository (include "home-lab-admin.imageTag" .) -}}
{{- end }}

{{- /*
The agent image, addressed the same way and for the same reason as the others.
It carries the agent's definition as well as the runtime, so the tag names the
prompt and the tool set and not only the binary.
*/ -}}
{{- define "home-lab-admin.agentImage" -}}
{{- printf "%s:%s" .Values.admin.agent.image.repository (include "home-lab-admin.imageTag" .) -}}
{{- end }}

{{- /*
The console's shell image: ttyd, tmux and the operator's tools. Its gate runs from
the panel image instead, because the gate's whole job is to read the panel's
session cookie, and the code that knows how is already in there.
*/ -}}
{{- define "home-lab-admin.consoleImage" -}}
{{- printf "%s:%s" .Values.admin.console.image.repository (include "home-lab-admin.imageTag" .) -}}
{{- end }}

{{- /*
What it takes to read the panel's session cookie and decide whether its holder may
write: the origin the cookie was issued for, the secret that seals it, and the
allowlist. The panel needs it to sign people in; the console's gate needs exactly
the same three to let them through, and two copies of this block are two places
for the gate to drift from the panel it is supposed to agree with.
*/ -}}
{{- define "home-lab-admin.authEnv" -}}
# The panel's own origin, and the base of the redirect URI eetr-auth must have
# registered. Auth.js builds the callback from it, so a value that disagrees with
# the registered URI fails at the callback rather than at sign-in, which is a
# confusing place to find out.
#
# https://<hostname> unless admin.web.authUrl overrides it. The override exists for
# a rig reached some other way than by its own hostname over TLS -- a local cluster
# behind a port-forward, where the origin the browser actually uses is
# http://localhost:3000 and the derived value is a URL nothing serves.
- name: AUTH_URL
  value: {{ .Values.admin.web.authUrl | default (printf "https://%s" (required "admin.web.hostname is required" .Values.admin.web.hostname)) | quote }}
# Seals the session cookie (JWE), which carries the operator's access and refresh
# tokens. Rotating it signs everybody out.
- name: AUTH_SECRET
  valueFrom:
    secretKeyRef:
      name: {{ .Values.admin.web.secret.name }}
      key: {{ .Values.admin.web.secret.authSecretKey }}
{{- with .Values.admin.web.writeEmails }}
- name: ADMIN_WRITE_EMAILS
  value: {{ . | quote }}
{{- end }}
# The console's rule for an empty allowlist: closed, unless this opens it. The
# panel needs it as much as the gate does, to say why the page has no terminal.
{{- if .Values.admin.console.allowAnyOperator }}
- name: ADMIN_CONSOLE_ALLOW_ANY_OPERATOR
  value: "true"
{{- end }}
{{- end }}
