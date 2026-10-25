<#--
  The username and password form: base's login.ftl with the same fields, ids
  and names, laid out like the web UI's forms.
-->
<#import "template.ftl" as layout>
<#import "passkeys.ftl" as passkeys>
<@layout.registrationLayout displayMessage=!messagesPerField.existsError('username','password') displayInfo=realm.password && realm.registrationAllowed && !registrationDisabled??; section>
    <#if section = "header">
        ${msg("loginAccountTitle")}
    <#elseif section = "subtitle">
        <#if client?? && client.name?has_content>to continue to ${advancedMsg(client.name)}<#else>to ${realm.displayName!realm.name}</#if>
    <#elseif section = "form">
        <div id="kc-form">
            <#if realm.password>
                <form id="kc-form-login" class="${properties.kcFormClass!}" onsubmit="login.disabled = true; return true;" action="${url.loginAction}" method="post" novalidate>
                    <#if !usernameHidden??>
                        <div class="${properties.kcFormGroupClass!}">
                            <label for="username" class="${properties.kcLabelClass!}"><#if !realm.loginWithEmailAllowed>${msg("username")}<#elseif !realm.registrationEmailAsUsername>${msg("usernameOrEmail")}<#else>${msg("email")}</#if></label>
                            <input tabindex="2" id="username" class="${properties.kcInputClass!}" name="username" value="${(login.username!'')}" type="text"
                                   autofocus autocomplete="${(enableWebAuthnConditionalUI?has_content)?then('username webauthn', 'username')}"
                                   aria-invalid="<#if messagesPerField.existsError('username','password')>true</#if>"
                                   dir="ltr"
                            />
                        </div>
                    </#if>

                    <div class="${properties.kcFormGroupClass!}">
                        <div class="m2m-label-row">
                            <label for="password" class="${properties.kcLabelClass!}">${msg("password")}</label>
                            <#if realm.resetPasswordAllowed>
                                <a tabindex="6" class="m2m-link-muted" href="${url.loginResetCredentialsUrl}">${msg("doForgotPassword")}</a>
                            </#if>
                        </div>
                        <div class="${properties.kcInputGroup!}" dir="ltr">
                            <input tabindex="3" id="password" class="${properties.kcInputClass!}" name="password" type="password" autocomplete="current-password"
                                   aria-invalid="<#if messagesPerField.existsError('username','password')>true</#if>"
                            />
                            <button class="${properties.kcFormPasswordVisibilityButtonClass!}" type="button" aria-label="${msg("showPassword")}"
                                    aria-controls="password" data-password-toggle tabindex="4"
                                    data-icon-show="${properties.kcFormPasswordVisibilityIconShow!}" data-icon-hide="${properties.kcFormPasswordVisibilityIconHide!}"
                                    data-label-show="${msg('showPassword')}" data-label-hide="${msg('hidePassword')}">
                                <i class="${properties.kcFormPasswordVisibilityIconShow!}" aria-hidden="true"></i>
                            </button>
                        </div>
                    </div>

                    <#if messagesPerField.existsError('username','password')>
                        <div class="${properties.kcAlertClass!} m2m-alert-error" id="input-error" role="alert" aria-live="polite">
                            <span class="${properties.kcFeedbackErrorIcon!}"></span>
                            <span class="${properties.kcAlertTitleClass!}">${kcSanitize(messagesPerField.getFirstError('username','password'))?no_esc}</span>
                        </div>
                    </#if>

                    <#if realm.rememberMe && !usernameHidden??>
                        <div class="${properties.kcFormGroupClass!}">
                            <label class="m2m-check">
                                <input tabindex="5" id="rememberMe" name="rememberMe" type="checkbox" class="${properties.kcCheckboxInputClass!}" <#if login.rememberMe??>checked</#if>>
                                ${msg("rememberMe")}
                            </label>
                        </div>
                    </#if>

                    <div id="kc-form-buttons" class="${properties.kcFormGroupClass!}">
                        <input type="hidden" id="id-hidden-input" name="credentialId" <#if auth.selectedCredential?has_content>value="${auth.selectedCredential}"</#if>/>
                        <input tabindex="7" class="${properties.kcButtonClass!} ${properties.kcButtonPrimaryClass!} ${properties.kcButtonBlockClass!} ${properties.kcButtonLargeClass!}" name="login" id="kc-login" type="submit" value="${msg("doLogIn")}"/>
                    </div>
                </form>
            </#if>
        </div>
        <@passkeys.conditionalUIData />
        <script type="module" src="${url.resourcesPath}/js/passwordVisibility.js"></script>
    <#elseif section = "info">
        <#if realm.password && realm.registrationAllowed && !registrationDisabled??>
            <p id="kc-registration">${msg("noAccount")} <a tabindex="8" href="${url.registrationUrl}">${msg("doRegister")}</a></p>
        </#if>
    <#elseif section = "socialProviders">
        <#if realm.password && social?? && social.providers?has_content>
            <div id="kc-social-providers" class="${properties.kcFormSocialAccountSectionClass!}">
                <p class="m2m-divider"><span>${msg("identity-provider-login-label")}</span></p>
                <ul class="${properties.kcFormSocialAccountListClass!}">
                    <#list social.providers as p>
                        <li>
                            <a data-once-link data-disabled-class="m2m-disabled" id="social-${p.alias}"
                               class="${properties.kcFormSocialAccountListButtonClass!}" type="button" href="${p.loginUrl}">
                                <span class="${properties.kcFormSocialAccountNameClass!}">${p.displayName!}</span>
                            </a>
                        </li>
                    </#list>
                </ul>
            </div>
        </#if>
    </#if>
</@layout.registrationLayout>
