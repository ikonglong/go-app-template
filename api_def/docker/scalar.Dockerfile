FROM public.ecr.aws/docker/library/nginx:alpine
WORKDIR /usr/share/nginx/html
COPY docker/scalar.index.html ./index.html
COPY docker/scalar.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
