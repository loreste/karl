# Guia de Solucion de Problemas

Esta guia cubre el diagnostico y la resolucion de problemas comunes con el Servidor de Medios Karl.

## Tabla de Contenidos

- [Herramientas de Diagnostico](#herramientas-de-diagnostico)
- [Problemas de Inicio](#problemas-de-inicio)
- [Problemas de Conexion](#problemas-de-conexion)
- [Problemas de Medios](#problemas-de-medios)
- [Problemas de Grabacion](#problemas-de-grabacion)
- [Problemas de Rendimiento](#problemas-de-rendimiento)
- [Problemas de Kubernetes](#problemas-de-kubernetes)
- [Obtener Ayuda](#obtener-ayuda)

---

## Herramientas de Diagnostico

### Verificar Estado de Karl

```bash
# Proceso en ejecucion
ps aux | grep karl

# Estado de systemd
systemctl status karl

# Estado de Docker
docker ps | grep karl
docker logs karl

# Estado de Kubernetes
kubectl get pods -l app=karl
kubectl describe pod -l app=karl
```

### Probar Protocolo NG

```bash
# Enviar comando ping
echo -n "d7:command4:pinge" | nc -u localhost 22222

# La respuesta esperada contiene "pong"
```

### Verificar Endpoints de Salud

```bash
# Salud simple
curl http://localhost:8086/health

# Salud detallada
curl http://localhost:8086/health/detail

# Sondas de Kubernetes
curl http://localhost:8086/startup
curl http://localhost:8086/live
curl http://localhost:8086/ready
```

### Ver Metricas

```bash
# Todas las metricas
curl http://localhost:9091/metrics

# Sesiones activas
curl -s http://localhost:9091/metrics | grep karl_sessions_active
```

### Verificar Logs

```bash
# Systemd
journalctl -u karl -f

# Docker
docker logs -f karl

# Kubernetes
kubectl logs -l app=karl -f

# Con nivel debug
KARL_LOG_LEVEL=debug ./karl
```

### Consultas API

```bash
# Listar sesiones
curl http://localhost:8080/api/v1/sessions

# Estadisticas del servidor
curl http://localhost:8080/api/v1/stats

# Sesion especifica
curl http://localhost:8080/api/v1/sessions/{session_id}
```

---

## Problemas de Inicio

### "bind: address already in use"

Otro proceso esta usando el puerto.

**Solucion**:
```bash
# Encontrar el proceso
sudo lsof -i :22222
sudo lsof -i :8080

# Terminar si es necesario
sudo kill <PID>

# O cambiar los puertos de Karl
KARL_NG_PORT=22223 ./karl
```

### "permission denied" para socket

Permisos insuficientes para socket Unix o puertos.

**Solucion**:
```bash
# Crear directorio con permisos correctos
sudo mkdir -p /var/run/karl
sudo chown karl:karl /var/run/karl

# Para puertos por debajo de 1024
sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/karl
```

### "too many open files"

Limite de descriptores de archivo demasiado bajo.

**Solucion**:
```bash
# Correccion temporal
ulimit -n 65535

# Correccion permanente - agregar a /etc/security/limits.conf
karl soft nofile 65535
karl hard nofile 65535

# O en servicio systemd
[Service]
LimitNOFILE=65535
```

### Archivo de Configuracion No Encontrado

```bash
# Verificar que el archivo existe
ls -la /etc/karl/config.json

# Verificar la ruta
KARL_CONFIG_PATH=/etc/karl/config.json ./karl

# Usar configuracion predeterminada
./karl  # Usa ./config/config.json
```

### Configuracion Invalida

```bash
# Validar JSON
cat /etc/karl/config.json | jq .

# Verificar problemas comunes
# - Comas faltantes
# - Comas finales
# - Valores invalidos
```

---

## Problemas de Conexion

### El Proxy SIP No Puede Conectar

**Sintomas**: Sin respuesta a comandos del protocolo NG

**Diagnostico**:
```bash
# Verificar que Karl esta escuchando
ss -ulnp | grep 22222

# Probar desde el proxy SIP
echo -n "d7:command4:pinge" | nc -u karl-ip 22222

# Verificar firewall
sudo iptables -L -n | grep 22222
```

**Soluciones**:
1. Verificar que Karl esta ejecutandose y escuchando en la interfaz correcta
2. Verificar que las reglas del firewall permiten UDP 22222
3. Verificar que la configuracion del proxy SIP apunta a la IP/puerto correctos

### La API No Responde

**Diagnostico**:
```bash
# Verificar que la API esta escuchando
ss -tlnp | grep 8080

# Probar localmente
curl http://localhost:8080/api/v1/stats

# Verificar desde remoto
curl http://karl-ip:8080/api/v1/stats
```

**Soluciones**:
1. Verificar que la API esta habilitada en la configuracion
2. Verificar firewall para TCP 8080
3. Verificar direccion de binding (use `0.0.0.0` para todas las interfaces)

### Fallo de Conexion a Redis

**Sintomas**: Las sesiones no se comparten entre instancias

**Diagnostico**:
```bash
# Probar conectividad de Redis
redis-cli -h redis-host ping

# Verificar logs de Karl
grep -i redis /var/log/karl.log

# Verificar configuracion
grep -A 5 '"database"' /etc/karl/config.json
```

**Soluciones**:
1. Verificar que Redis esta ejecutandose
2. Verificar conectividad de red
3. Verificar direccion de Redis en la configuracion
4. Verificar autenticacion de Redis si esta habilitada

---

## Problemas de Medios

### Sin Audio (Ambas Direcciones)

**Diagnostico**:
```bash
# Verificar que las sesiones se crean
curl http://localhost:8080/api/v1/sessions

# Verificar que los puertos RTP estan asignados
ss -ulnp | grep -E "3[0-9]{4}"

# Verificar contadores de paquetes
curl http://localhost:9091/metrics | grep packets
```

**Causas Comunes**:
1. Puertos RTP bloqueados por firewall
2. Configuracion incorrecta de `media_ip`
3. Problemas de NAT — IP privada filtrándose en SDP enviado a peers externos
4. `interfaces` / `direction` faltantes o mal configuradas — si su proxy SIP envia `direction=internal direction=external` pero Karl no tiene interfaces coincidentes configuradas, recurre a `media_ip` que puede ser una direccion privada

**Soluciones**:
```bash
# Abrir puertos RTP
sudo iptables -A INPUT -p udp --dport 30000:40000 -j ACCEPT

# Verificar media_ip
KARL_MEDIA_IP=192.168.1.100 ./karl

# Para NAT, establecer public_ip
KARL_PUBLIC_IP=203.0.113.50 ./karl
```

Si usa interfaces internas/externas separadas, verifique:
1. La configuracion de Karl tiene `interfaces` con `address` (binding) y `advertise_addr` (SDP) correctos para cada interfaz nombrada
2. Su proxy SIP envia el flag `direction` coincidiendo con esos nombres de interfaz
3. El `advertise_addr` para la interfaz externa es la IP publica/Elastica accesible por los carriers

Vea [Configuracion — Interfaces Nombradas](../configuration.md#interfaces-nombradas) para detalles de configuracion.

### Audio Unidireccional

**Causas Comunes**:
1. NAT asimetrico
2. Firewall bloqueando trafico de retorno
3. Manipulacion incorrecta de SDP

**Soluciones**:
```bash
# En el proxy SIP, forzar RTP simetrico
rtpengine_manage("... symmetric");

# Verificar que ambas direcciones tienen paquetes
curl http://localhost:8080/api/v1/sessions/{id}
# Verificar packets_sent y packets_received > 0
```

### Mala Calidad de Audio

**Diagnostico**:
```bash
# Verificar metricas RTCP
curl http://localhost:9091/metrics | grep rtcp

# Buscar:
# - Alta perdida de paquetes (> 5%)
# - Alto jitter (> 50ms)
# - Alto RTT (> 200ms)
```

**Soluciones**:
1. Habilitar/aumentar redundancia FEC
2. Aumentar tamano del jitter buffer
3. Verificar la ruta de red en busca de problemas

```json
{
  "fec": {
    "enabled": true,
    "redundancy": 0.30,
    "adaptive_mode": true
  },
  "jitter_buffer": {
    "enabled": true,
    "max_delay": 200,
    "adaptive_mode": true
  }
}
```

### Las Sesiones No Se Crean

**Diagnostico**:
```bash
# Verificar logs del protocolo NG
grep -i "offer\|answer" /var/log/karl.log

# Verificar que se esta enviando SDP
tcpdump -i any port 22222 -A
```

**Causas Comunes**:
1. SDP invalido en la solicitud
2. Parametros requeridos faltantes
3. Limite de sesiones alcanzado

**Soluciones**:
1. Verificar que el proxy SIP esta enviando solicitudes completas
2. Verificar que call-id y from-tag estan presentes
3. Aumentar max_sessions si es necesario

---

## Problemas de Grabacion

### Las Grabaciones No Se Crean

**Diagnostico**:
```bash
# Verificar que la grabacion esta habilitada
grep -A 10 '"recording"' /etc/karl/config.json

# Verificar que el directorio existe y es escribible
ls -la /var/lib/karl/recordings
touch /var/lib/karl/recordings/test.txt

# Verificar mensajes de grabacion
grep -i recording /var/log/karl.log
```

**Soluciones**:
1. Habilitar grabacion en la configuracion
2. Crear directorio con permisos correctos
3. Asegurar que el flag `record-call` es enviado por el proxy SIP

### Archivos de Grabacion Vacios

**Causas**:
1. No se recibieron paquetes de medios
2. Codec no soportado
3. La sesion termino demasiado rapido

**Diagnostico**:
```bash
# Verificar que la sesion tuvo medios
curl http://localhost:8080/api/v1/sessions/{id}
# Verificar packets_received > 0

# Verificar codec
grep -i codec /var/log/karl.log
```

### Problemas de Espacio en Disco

```bash
# Verificar uso de disco
df -h /var/lib/karl/recordings

# Encontrar archivos grandes
du -sh /var/lib/karl/recordings/*

# Habilitar politica de retencion
{
  "recording": {
    "retention_days": 7
  }
}

# Limpieza manual
find /var/lib/karl/recordings -name "*.wav" -mtime +7 -delete
```

---

## Problemas de Rendimiento

### Alto Uso de CPU

**Diagnostico**:
```bash
# Verificar CPU del proceso
top -p $(pgrep karl)

# Perfil (si se compilo con profiling)
curl http://localhost:9091/debug/pprof/profile?seconds=30 > profile.pb.gz
```

**Causas Comunes**:
1. Demasiadas sesiones concurrentes
2. Sobrecarga de FEC demasiado alta
3. Recursos insuficientes

**Soluciones**:
1. Escalar horizontalmente
2. Reducir redundancia FEC
3. Aumentar asignacion de CPU

### Alto Uso de Memoria

**Diagnostico**:
```bash
# Verificar memoria
ps aux | grep karl

# Metricas de memoria
curl http://localhost:9091/metrics | grep memory
```

**Soluciones**:
1. Reducir max_sessions
2. Disminuir tamano del jitter buffer
3. Agregar limites de memoria y dejar que el orquestador maneje los reinicios

### Creacion Lenta de Sesiones

**Diagnostico**:
```bash
# Verificar latencia de comandos
curl http://localhost:9091/metrics | grep ng_command_duration
```

**Soluciones**:
1. Verificar latencia de Redis (si usa Redis)
2. Reducir rango de asignacion de puertos
3. Escalar horizontalmente

---

## Problemas de Kubernetes

### El Pod No Inicia

```bash
# Verificar estado del pod
kubectl describe pod -l app=karl

# Problemas comunes:
# - Errores de descarga de imagen
# - Limites de recursos
# - Fallos de sondas
# - Problemas de montaje de volumenes
```

### Fallan las Sondas

```bash
# Probar sondas manualmente
kubectl exec -it <pod> -- curl localhost:8086/startup
kubectl exec -it <pod> -- curl localhost:8086/live
kubectl exec -it <pod> -- curl localhost:8086/ready

# Verificar configuracion de sondas
kubectl get pod <pod> -o yaml | grep -A 10 "Probe"
```

### Sin Conectividad de Red

```bash
# Verificar endpoints del servicio
kubectl get endpoints karl

# Probar desde dentro del cluster
kubectl run test --rm -it --image=alpine -- sh
apk add curl netcat-openbsd
nc -u karl 22222

# Verificar politicas de red
kubectl get networkpolicies
```

### Problemas con hostNetwork

```bash
# Verificar que hostNetwork esta habilitado
kubectl get pod <pod> -o yaml | grep hostNetwork

# Verificar IP del nodo
kubectl get pod <pod> -o wide

# Verificar que no hay conflictos de puertos en el nodo
kubectl exec -it <pod> -- ss -ulnp
```

---

## Obtener Ayuda

### Informacion a Recopilar

Al reportar problemas, incluya:

1. **Version de Karl**
```bash
./karl --version
```

2. **Configuracion** (sanitizar datos sensibles)
```bash
cat /etc/karl/config.json
```

3. **Logs**
```bash
journalctl -u karl --since "1 hour ago" > karl-logs.txt
```

4. **Metricas**
```bash
curl http://localhost:9091/metrics > metrics.txt
```

5. **Entorno**
```bash
uname -a
cat /etc/os-release
```

### Canales de Soporte

- **GitHub Issues**: [github.com/loreste/karl/issues](https://github.com/loreste/karl/issues)
- **GitHub Discussions**: [github.com/loreste/karl/discussions](https://github.com/loreste/karl/discussions)

### Resumen de Comandos Utiles

```bash
# Verificacion rapida de salud
curl -s http://localhost:8086/health | jq .

# Conteo de sesiones
curl -s http://localhost:9091/metrics | grep karl_sessions_active

# Errores recientes
grep -i error /var/log/karl.log | tail -20

# Probar protocolo NG
echo -n "d7:command4:pinge" | nc -u localhost 22222

# Verificacion de puertos
ss -ulnp | grep -E "22222|3[0-9]{4}"
```

---

## Siguientes Pasos

- [Referencia de Configuracion](../configuration.md)
- [Configuracion de Monitoreo](./monitoring-prometheus.md)
- [Despliegue en Kubernetes](./deploying-kubernetes.md)
