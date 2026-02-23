FROM nginx:alpine3.23

COPY ./html /usr/share/nginx/html

RUN chmod 755 /etc/nginx/conf.d/default.conf && rm -f /etc/nginx/conf.d/default.conf
COPY ./nginx/nginx.conf /etc/nginx/conf.d/

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]