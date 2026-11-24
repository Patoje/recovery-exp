# Misma imagen de Gitea, con el ejecutable instalado en otra ruta.
FROM gitea/gitea@sha256:aa90e8f00b755b0d665214ab9c14447ea44d3190dbe6b6d0cb46cd9c4f55d309
RUN mkdir -p /opt/gitea/bin \
 && mv /usr/local/bin/gitea /opt/gitea/bin/gitea \
 && sed -i 's#/usr/local/bin/gitea#/opt/gitea/bin/gitea#' /etc/s6/gitea/run
