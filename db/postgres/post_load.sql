{{ if not .Logged }}
ALTER TABLE {{ .CompanyTableFullName }} SET LOGGED;
ALTER TABLE empresa SET LOGGED;
ALTER TABLE estabelecimento SET LOGGED;
ALTER TABLE simples SET LOGGED;
ALTER TABLE socio SET LOGGED;
ALTER TABLE estabelecimento_cnae SET LOGGED;
ALTER TABLE regime_tributario SET LOGGED;
{{ end }}
