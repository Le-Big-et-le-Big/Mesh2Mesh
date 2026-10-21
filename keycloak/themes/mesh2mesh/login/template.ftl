<#--
  The page around every Mesh2Mesh login screen: base's template.ftl with the
  web UI's look. The scripts, nested sections and forms are base's, unchanged,
  so every page that extends it (login, password reset, OTP, logout, errors)
  keeps working.
-->
<#import "footer.ftl" as loginFooter>
<#import "theme-resources.ftl" as themeResourceTags>
<#macro registrationLayout bodyClass="" displayInfo=false displayMessage=true displayRequiredFields=false>
<!DOCTYPE html>
<html class="${properties.kcHtmlClass!}" lang="${lang}"<#if realm.internationalizationEnabled> dir="${(locale.rtl)?then('rtl','ltr')}"</#if>>

<head>
    <meta charset="utf-8">
    <meta http-equiv="Content-Type" content="text/html; charset=UTF-8" />
    <meta name="color-scheme" content="light dark" />
    <#if properties.meta?has_content>
        <#list properties.meta?split(' ') as meta>
            <meta name="${meta?split('==')[0]}" content="${meta?split('==')[1]}"/>
        </#list>
    </#if>
    <title>${title!}</title>
    <link rel="icon" type="image/svg+xml" href="${url.resourcesPath}/img/logo.svg" />
    <#if themeResources?? && themeResources.styles?has_content>
        <@themeResourceTags.renderStyles themeResources.styles url.resourcesPath />
    <#elseif properties.styles?has_content>
        <#list properties.styles?split(' ') as style>
            <link href="${url.resourcesPath}/${style}" rel="stylesheet" />
        </#list>
    </#if>
    <#if themeResources?? && themeResources.scripts?has_content>
        <@themeResourceTags.renderScripts themeResources.scripts url.resourcesPath "text/javascript" />
    <#elseif properties.scripts?has_content>
        <#list properties.scripts?split(' ') as script>
            <script src="${url.resourcesPath}/${script}" type="text/javascript"></script>
        </#list>
    </#if>
    <script type="importmap">
        {
            "imports": {
                "rfc4648": "${url.resourcesCommonPath}/vendor/rfc4648/rfc4648.js"
            }
        }
    </script>
    <script src="${url.resourcesPath}/js/menu-button-links.js" type="module"></script>
    <#if scripts??>
        <#list scripts as script>
            <script src="${script}" type="text/javascript"></script>
        </#list>
    </#if>
    <script type="module">
        <#outputformat "JavaScript">
        import { startSessionPolling } from ${(url.resourcesPath + "/js/authChecker.js")?c};

        startSessionPolling(
            ${url.ssoLoginInOtherTabsUrl?c}
        );
        </#outputformat>
    </script>
    <script type="module">
        document.addEventListener("click", (event) => {
            const link = event.target.closest("a[data-once-link]");

            if (!link) {
                return;
            }

            if (link.getAttribute("aria-disabled") === "true") {
                event.preventDefault();
                return;
            }

            const { disabledClass } = link.dataset;

            if (disabledClass) {
                link.classList.add(...disabledClass.trim().split(/\s+/));
            }

            link.setAttribute("role", "link");
            link.setAttribute("aria-disabled", "true");
        });
    </script>
    <#if authenticationSession??>
        <script type="module">
            <#outputformat "JavaScript">
            import { checkAuthSession } from ${(url.resourcesPath + "/js/authChecker.js")?c};

            checkAuthSession(
                ${authenticationSession.authSessionIdHash?c}
            );
            </#outputformat>
        </script>
    </#if>
</head>

<body class="${properties.kcBodyClass!} ${bodyClass}" data-page-id="login-${pageId}">
<div class="${properties.kcLoginClass!}">
    <header class="m2m-topbar">
        <#if client?? && client.baseUrl?has_content>
            <a class="m2m-brand" href="${client.baseUrl}" aria-label="Mesh2Mesh home"><@logo/></a>
        <#else>
            <span class="m2m-brand"><@logo/></span>
        </#if>

        <#if realm.internationalizationEnabled && locale.supported?size gt 1>
            <div class="${properties.kcLocaleMainClass!}" id="kc-locale">
                <div id="kc-locale-wrapper">
                    <div id="kc-locale-dropdown" class="menu-button-links">
                        <button tabindex="1" id="kc-current-locale-link" aria-label="${msg("languages")}" aria-haspopup="true" aria-expanded="false" aria-controls="language-switch1">${locale.current}</button>
                        <ul role="menu" tabindex="-1" aria-labelledby="kc-current-locale-link" aria-activedescendant="" id="language-switch1" class="${properties.kcLocaleListClass!}">
                            <#assign i = 1>
                            <#list locale.supported as l>
                                <li role="none">
                                    <a role="menuitem" id="language-${i}" class="${properties.kcLocaleItemClass!}" href="${l.url}">${l.label}</a>
                                </li>
                                <#assign i++>
                            </#list>
                        </ul>
                    </div>
                </div>
            </div>
        </#if>
    </header>

    <main class="m2m-main">
        <div class="m2m-column">
            <div class="${properties.kcFormCardClass!}">
                <header class="${properties.kcFormHeaderClass!}">
                    <h1 id="kc-page-title"><#nested "header"></h1>
                    <p class="m2m-subtitle"><#nested "subtitle"></p>

                    <#if auth?has_content && auth.showUsername() && !auth.showResetCredentials()>
                        <#nested "show-username">
                        <div id="kc-username" class="m2m-username">
                            <#assign who = auth.attemptedUsername!"">
                            <span class="m2m-avatar" aria-hidden="true"><#if who?has_content>${who?substring(0, 1)?upper_case}</#if></span>
                            <label id="kc-attempted-username">${auth.attemptedUsername}</label>
                            <a id="reset-login" href="${url.loginRestartFlowUrl}" aria-label="${msg("restartLoginTooltip")}">${msg("restartLoginTooltip")}</a>
                        </div>
                    </#if>

                    <#if displayRequiredFields>
                        <p class="m2m-required"><span class="required">*</span> ${msg("requiredFields")}</p>
                    </#if>
                </header>

                <div id="kc-content">
                    <div id="kc-content-wrapper">
                        <#-- App-initiated actions should not see warnings about completing the action during login. -->
                        <#if displayMessage && message?has_content && (message.type != 'warning' || !isAppInitiatedAction??)>
                            <div class="${properties.kcAlertClass!} m2m-alert-${message.type}" role="alert">
                                <#if message.type = 'success'><span class="${properties.kcFeedbackSuccessIcon!}"></span></#if>
                                <#if message.type = 'warning'><span class="${properties.kcFeedbackWarningIcon!}"></span></#if>
                                <#if message.type = 'error'><span class="${properties.kcFeedbackErrorIcon!}"></span></#if>
                                <#if message.type = 'info'><span class="${properties.kcFeedbackInfoIcon!}"></span></#if>
                                <span class="${properties.kcAlertTitleClass!}">${kcSanitize(message.summary)?no_esc}</span>
                            </div>
                        </#if>

                        <#nested "form">

                        <#if auth?has_content && auth.showTryAnotherWayLink()>
                            <form id="kc-select-try-another-way-form" action="${url.loginAction}" method="post">
                                <div class="${properties.kcFormGroupClass!} m2m-center">
                                    <input type="hidden" name="tryAnotherWay" value="on"/>
                                    <a href="#" id="try-another-way"
                                       onclick="document.forms['kc-select-try-another-way-form'].requestSubmit();return false;">${msg("doTryAnotherWay")}</a>
                                </div>
                            </form>
                        </#if>

                        <#if switchOrganizationEnabled?? && switchOrganizationEnabled>
                            <form id="kc-switch-organization-form" action="${url.loginAction}" method="post">
                                <div class="${properties.kcFormGroupClass!} m2m-center">
                                    <input type="hidden" name="switchOrganization" value="true"/>
                                    <a href="#" id="switch-organization"
                                       onclick="document.forms['kc-switch-organization-form'].requestSubmit();return false;">${msg("doSwitchOrganization")}</a>
                                </div>
                            </form>
                        </#if>

                        <#nested "socialProviders">

                        <#if displayInfo>
                            <div id="kc-info" class="${properties.kcSignUpClass!}">
                                <div id="kc-info-wrapper">
                                    <#nested "info">
                                </div>
                            </div>
                        </#if>
                    </div>
                </div>

                <@loginFooter.content/>
            </div>

            <#if client?? && client.baseUrl?has_content>
                <p class="m2m-back"><a href="${client.baseUrl}">&larr; ${msg("backToApplication")}</a></p>
            </#if>
        </div>
    </main>
</div>
</body>
</html>
</#macro>

<#-- The Mesh2Mesh mark and wordmark, as in the web UI's logo.tsx. -->
<#macro logo>
    <svg class="m2m-mark" viewBox="0 0 32 32" aria-hidden="true">
        <rect width="32" height="32" rx="8" class="m2m-mark-bg"/>
        <path d="M10 21.5 16 10.5 22 21.5Z" fill="none" class="m2m-mark-line" stroke-width="2" stroke-linecap="round"/>
        <g class="m2m-mark-dot">
            <circle cx="16" cy="10.5" r="3"/>
            <circle cx="10" cy="21.5" r="3"/>
            <circle cx="22" cy="21.5" r="3"/>
        </g>
    </svg>
    <span class="m2m-wordmark">Mesh2Mesh</span>
</#macro>
