# Primeros Pasos con el Servidor de Medios Karl

Esta guia le ayudara a poner Karl en funcionamiento en menos de 5 minutos.

## Requisitos Previos

- Go 1.26 o superior (para compilar desde el codigo fuente)
- Docker (opcional, para despliegue en contenedores)
- Un proxy SIP (OpenSIPS o Kamailio) para uso en produccion

## Opcion 1: Ejecutar desde el Codigo Fuente

```bash
# Clonar el repositorio
git clone https://github.com/loreste/karl.git
cd karl

# Compilar
go build -o karl

# Ejecutar con la configuracion predeterminada
./karl
```

Karl esta ahora ejecutandose con la configuracion predeterminada:
- Protocolo NG: Puerto UDP 22222
- Puertos RTP: 30000-40000
- API REST: Puerto TCP 8080
- Health Check: Puerto TCP 8086
- Metricas: Puerto TCP 9091

## Opcion 2: Ejecutar con Docker

```bash
# Ejecutar con red del host (recomendado para servidores de medios)
docker run -d \
  --name karl \
  --network host \
  loreste/karl:latest
```

O con mapeo explicito de puertos:

```bash
docker run -d \
  --name karl \
  -p 22222:22222/udp \
  -p 30000-30100:30000-30100/udp \
  -p 8080:8080 \
  -p 8086:8086 \
  -p 9091:9091 \
  loreste/karl:latest
```

## Verificar la Instalacion

### Probar el Protocolo NG

```bash
# Enviar un comando ping
echo -n "d7:command4:pinge" | nc -u localhost 22222
```

La respuesta esperada contiene `pong`.

### Verificar el Endpoint de Salud

```bash
curl http://localhost:8086/health
```

Respuesta esperada:
```json
{"status":"UP"}
```

### Ver Metricas

```bash
curl http://localhost:9091/metrics | head -20
```

## Conectar a su Proxy SIP

### OpenSIPS

Agregue a su `opensips.cfg`:

```opensips
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
```

### Kamailio

Agregue a su `kamailio.cfg`:

```kamailio
loadmodule "rtpengine.so"
modparam("rtpengine", "rtpengine_sock", "udp:127.0.0.1:22222")
```

## Realizar una Llamada de Prueba

1. Registre dos telefonos SIP con su proxy SIP
2. Realice una llamada entre ellos
3. Verifique que el medio fluye a traves de Karl:

```bash
# Verificar sesiones activas
curl http://localhost:8080/api/v1/sessions
```

## Siguientes Pasos

- [Referencia de Configuracion](./configuration.md) - Personalizar Karl para su entorno
- [Despliegue en Kubernetes](./how-to/deploying-kubernetes.md) - Desplegar en Kubernetes
- [Configurar Monitoreo](./how-to/monitoring-prometheus.md) - Agregar Prometheus y Grafana
- [Habilitar Grabacion](./how-to/setting-up-recording.md) - Grabar llamadas

## Problemas Comunes

### "connection refused" en el puerto 22222

Karl no esta ejecutandose o esta vinculado a una interfaz diferente. Verifique:

```bash
# Esta Karl ejecutandose?
ps aux | grep karl

# En que puerto esta escuchando?
ss -ulnp | grep 22222
```

### Sin audio en las llamadas

1. Verifique que los puertos RTP (30000-40000) estan accesibles
2. Verifique que las reglas del firewall permiten trafico UDP
3. Revise los logs de Karl en busca de errores:

```bash
# Si se ejecuta como servicio
journalctl -u karl -f

# Si se ejecuta en Docker
docker logs -f karl
```

Consulte la [Guia de Solucion de Problemas](./how-to/troubleshooting.md) para mas ayuda.
