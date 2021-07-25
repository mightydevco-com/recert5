---
title: "Getting Started"
date: 2021-07-25T11:51:01-05:00
draft: false
---

# GETTING STARTED
Before you start make sure you have the following:

>kubectl, helm and curl command line tools

>access to a Kubernetes cluster (preferably non-production)

>access to a domain or subdomain (you must be able to configure the DNS entries)

## INSTALL THE OPERATOR

To install the operator open a terminal and run this:

```bash
curl http://uberscott.com/recert5/files/recert5-operator.yaml | kubectl apply -f -
```

The operator will install in a namespace called 'recert5-system'

check to make sure everything is up and running before you move on

```bash
kubectl get pods -n recert5-system                                                                            195ms  Sun Jul 25 20:19:51 2021
NAME                                          READY   STATUS    RESTARTS   AGE
recert5-controller-manager-795b888849-lp9ht   2/2     Running   0          29s
```

Notice that the controller manager is in the Running STATUS.

## DOWNLOAD THE HELM CHARTS

```bash
curl http://uberscott.com/recert5/files/helm-charts.zip --output helm-charts.zip
```

## UNZIP THE HELM CHARTS

```bash
unzip helm-charts.zip
```

## INSTALL A MOCK NGINX WEBSITE
This will serve as our mock website.

In the newly unzipped helm directory:

```bash
helm install example nginx
```

Check if it is running...

## INSTALL RECERT SSL REVERSE PROXY

```bash
helm install ssl-reverse-proxy ssl-reverse-proxy
```

Here is the YAML file that the helm chart is installing:

```yaml
apiVersion: recert5.uberscott.com/v1
kind: RecertSSLReverseProxy
metadata:
  name: example
spec:
  pass: "http://example-nginx:80"
  replicas: 1
```

As you can see it is creating a RecertSSLReverseProxy CR which passes traffic to http://nginx-example:80.


## FIND THE REVERSE PROXY EXTERNAL IP ADDRESS

Get the details for the 'service example-nginx-ssl-reverse-proxy' service:

```bash
kubectl get service example-nginx-ssl-reverse-proxy
```

The output should look something like this: 

```bash
NAME                              TYPE           CLUSTER-IP   EXTERNAL-IP     PORT(S)                      AGE
example-nginx-ssl-reverse-proxy   LoadBalancer   10.0.7.13    34.134.55.212   80:30783/TCP,443:30321/TCP   80s
```

NOTE: The EXTERNAL-IP may be in a pending state for several minutes as a static IP is provisioned.

Keep a copy the EXTERNAL-IP address for the next step.  

## SET DNS RECORDS FOR YOUR TEST DOMAIN
Now you will need to modify your DNS records for the test domain you are using.  In our example we are using `example.uberscott.com`

In this example we would make a new A record for example.uberscott.com and point it to the external IP address that the service assigned: '34.134.55.212'

## CHECK DNS RESOLUTION
DNS propogation can take days, however, if you selected a subdomain that wasn't in use before like "recert5-test.my-domain.com" in my experience it can resolve immediately since there should be no caching conflicts.

```bash
curl http://example.uberscott.com/
```

Notice we are just testing HTTP traffic at this point, which should resolve to the Nginx example webpage we setup earlier.

you shoul see an output of the default nginx page:
```html
<!DOCTYPE html>
<html>
<head>
<title>Welcome to nginx!</title>
<style>
    body {
        width: 35em;
        margin: 0 auto;
        font-family: Tahoma, Verdana, Arial, sans-serif;
    }
</style>
</head>
<body>
<h1>Welcome to nginx!</h1>
<p>If you see this page, the nginx web server is successfully installed and
working. Further configuration is required.</p>

<p>For online documentation and support please refer to
<a href="http://nginx.org/">nginx.org</a>.<br/>
Commercial support is available at
<a href="http://nginx.com/">nginx.com</a>.</p>

<p><em>Thank you for using nginx.</em></p>
</body>
</html>
```



## CREATE A RECERT CR
Lastly we create the Recert CR which will bind the domain to the ReverseSSLProxy.

to run this helm chart you must pass YOUR domain and YOUR email as a parameterized value.

```bash
helm install recert recert --set Domain=example.uberscott.com --set Email=scott@mightydevco.com
```

The email address is so that Recert5 can send and email if the recertification process fails. 

Again, let's look under the hood at the yaml file we just generated via our helm chart:

```yaml
apiVersion: recert5.uberscott.com/v1
kind: Recert
metadata:
  name: example
spec:
  domain: {{ .Values.Domain }}
  email: {{ .Values.Email }}
  sslReverseProxy: example
```

As you can see it takes the Domain and Email as a templated value, but also we are pointing this Recert to the sslReverseProxy named 'example' which we created earlier.

The certification process can take several minutes to complete.  In the meantime you can periodically check the status of the recertification process  like this:

```bash
kubectl get recert.recert5.uberscott.com -o=jsonpath="{.items[0].status.state}"
```

which will return a status of Pending, Creating, Failed or Updated.

When the status is Updated, that means the certification process succeeded.  

It may then take a few more seconds for the SSLReverseProxies to restart with the new certificates.

You can now run the same curl test you ran before on the HTTP using HTTPS:

```bash
curl https://example.uberscott.com/
```

And you should get an identical result as HTTP:


```html
<!DOCTYPE html>
<html>
<head>
<title>Welcome to nginx!</title>
<style>
    body {
        width: 35em;
        margin: 0 auto;
        font-family: Tahoma, Verdana, Arial, sans-serif;
    }
</style>
</head>
<body>
<h1>Welcome to nginx!</h1>
<p>If you see this page, the nginx web server is successfully installed and
working. Further configuration is required.</p>

<p>For online documentation and support please refer to
<a href="http://nginx.org/">nginx.org</a>.<br/>
Commercial support is available at
<a href="http://nginx.com/">nginx.com</a>.</p>

<p><em>Thank you for using nginx.</em></p>
</body>
</html>
```
























