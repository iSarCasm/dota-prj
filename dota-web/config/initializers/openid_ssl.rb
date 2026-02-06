# frozen_string_literal: true

# Ensure Ruby OpenID (used by omniauth-openid / omniauth-steam) can verify HTTPS.
#
# On some macOS/Homebrew setups, OpenSSL's default cert directory
# (`OpenSSL::X509::DEFAULT_CERT_DIR`) may be empty, which causes Ruby OpenID to
# warn "no CA path was specified" and/or fail SSL verification.
#
# Point Ruby OpenID at the CA bundle file provided by Homebrew OpenSSL.
if defined?(OpenID) && defined?(OpenSSL)
  begin
    ca_file = OpenSSL::X509::DEFAULT_CERT_FILE

    if ca_file && File.file?(ca_file) && OpenID.respond_to?(:fetcher) && OpenID.fetcher.respond_to?(:ca_file=)
      OpenID.fetcher.ca_file = ca_file
    end
  rescue StandardError
    # If OpenID/OpenSSL isn't fully loaded yet, do nothing.
  end
end
