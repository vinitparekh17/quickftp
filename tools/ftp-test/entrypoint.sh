#!/bin/sh
set -eu

: "${FTP_USER:=quickftp}"
: "${FTP_PASSWORD:=quickftp-test-only}"
: "${FTP_PORT:=21}"
: "${FTP_IMPLICIT:=NO}"
: "${PASV_MIN_PORT:=30000}"
: "${PASV_MAX_PORT:=30009}"

if ! id "$FTP_USER" >/dev/null 2>&1; then
  useradd --home-dir /srv/ftp --no-create-home --shell /bin/bash "$FTP_USER"
fi
printf '%s:%s\n' "$FTP_USER" "$FTP_PASSWORD" | chpasswd
chown -R root:root /srv/ftp
chmod 755 /srv/ftp
mkdir -p /var/run/vsftpd/empty
chown root:root /var/run/vsftpd/empty
chmod 555 /var/run/vsftpd/empty
mkdir -p /srv/ftp/site/wwwroot
chown -R "$FTP_USER":"$FTP_USER" /srv/ftp/site

openssl req -x509 -nodes -newkey rsa:2048 -days 365 \
  -keyout /etc/ssl/private/vsftpd.key \
  -out /etc/ssl/certs/vsftpd.crt \
  -subj "/CN=localhost" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" >/dev/null 2>&1
chmod 600 /etc/ssl/private/vsftpd.key

cat >/etc/vsftpd.conf <<EOF
listen=YES
listen_ipv6=NO
listen_port=$FTP_PORT
anonymous_enable=NO
local_enable=YES
write_enable=YES
local_umask=022
chroot_local_user=YES
allow_writeable_chroot=NO
local_root=/srv/ftp
seccomp_sandbox=NO
pasv_enable=YES
pasv_min_port=$PASV_MIN_PORT
pasv_max_port=$PASV_MAX_PORT
pasv_address=127.0.0.1
ssl_enable=YES
allow_anon_ssl=NO
force_local_logins_ssl=YES
force_local_data_ssl=YES
require_ssl_reuse=NO
implicit_ssl=$FTP_IMPLICIT
xferlog_enable=YES
xferlog_std_format=NO
vsftpd_log_file=/var/log/vsftpd.log
debug_ssl=YES
rsa_cert_file=/etc/ssl/certs/vsftpd.crt
rsa_private_key_file=/etc/ssl/private/vsftpd.key
ssl_sslv2=NO
ssl_sslv3=NO
ssl_tlsv1=YES
EOF

exec /usr/sbin/vsftpd /etc/vsftpd.conf
