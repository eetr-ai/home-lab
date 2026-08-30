{{- define "home-lab-admin.labels" -}}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: home-lab-admin
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end }}

{{- /*
The tag all three images carry.

One tag rather than three because one pipeline builds all three from one commit,
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
The agent image, addressed the same way and for the same reason as the other two.
It carries the agent's definition as well as the runtime, so the tag names the
prompt and the tool set and not only the binary.
*/ -}}
{{- define "home-lab-admin.agentImage" -}}
{{- printf "%s:%s" .Values.admin.agent.image.repository (include "home-lab-admin.imageTag" .) -}}
{{- end }}
