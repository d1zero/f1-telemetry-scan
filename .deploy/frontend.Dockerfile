FROM nginx:alpine3.23

COPY ./html /usr/share/nginx/html

RUN if [ -f /etc/nginx/conf.d/default.conf ]; then rm /etc/nginx/conf.d/default.conf; fi
COPY ./nginx/nginx.conf /etc/nginx/conf.d/

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]