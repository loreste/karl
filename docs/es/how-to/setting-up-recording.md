# Como Configurar la Grabacion de Llamadas

Esta guia cubre la configuracion y gestion de la grabacion de llamadas en el Servidor de Medios Karl.

## Tabla de Contenidos

- [Resumen](#resumen)
- [Habilitar la Grabacion](#habilitar-la-grabacion)
- [Modos de Grabacion](#modos-de-grabacion)
- [Configuracion de Almacenamiento](#configuracion-de-almacenamiento)
- [Activar la Grabacion](#activar-la-grabacion)
- [Gestionar Grabaciones](#gestionar-grabaciones)
- [Consideraciones de Produccion](#consideraciones-de-produccion)
- [Solucion de Problemas](#solucion-de-problemas)

---

## Resumen

Karl soporta grabacion de llamadas de grado profesional con:

- Multiples modos de grabacion (mixed, stereo, separate)
- Formato de salida WAV (PCM de 16 bits)
- Transcodificacion automatica de codecs
- Gestion de politicas de retencion
- API REST para control de grabacion

---

## Habilitar la Grabacion

### Archivo de Configuracion

```json
{
  "recording": {
    "enabled": true,
    "base_path": "/var/lib/karl/recordings",
    "format": "wav",
    "mode": "stereo",
    "sample_rate": 8000,
    "bits_per_sample": 16,
    "max_file_size": 104857600,
    "retention_days": 30
  }
}
```

### Variables de Entorno

```bash
export KARL_RECORDING_ENABLED=true
export KARL_RECORDING_PATH=/var/lib/karl/recordings
```

### Verificar Configuracion

```bash
# Verificar que la grabacion esta habilitada
curl http://localhost:8080/api/v1/stats | jq .recording_enabled
```

---

## Modos de Grabacion

### Modo Mixed

Ambas partes combinadas en un unico archivo mono.

```json
{
  "recording": {
    "mode": "mixed"
  }
}
```

**Salida**: Archivo WAV mono unico
**Caso de uso**: Archivo simple, cumplimiento normativo

### Modo Stereo

Llamante en canal izquierdo, llamado en canal derecho.

```json
{
  "recording": {
    "mode": "stereo"
  }
}
```

**Salida**: Archivo WAV stereo unico
**Caso de uso**: Analisis de calidad, identificacion de hablante

### Modo Separate

Cada parte en un archivo separado.

```json
{
  "recording": {
    "mode": "separate"
  }
}
```

**Salida**: Dos archivos WAV mono (uno por parte)
**Caso de uso**: Post-procesamiento, servicios de transcripcion

---

## Configuracion de Almacenamiento

### Estructura de Directorios

Las grabaciones se almacenan con esta estructura:

```
/var/lib/karl/recordings/
├── 2024-01-15/
│   ├── call-abc123_1705312200_stereo.wav
│   ├── call-def456_1705312500_stereo.wav
│   └── call-ghi789_1705313000_stereo.wav
└── 2024-01-16/
    └── ...
```

### Nomenclatura de Archivos

Formato: `{call_id}_{unix_timestamp}_{mode}.wav`

Ejemplo: `call-abc123_1705312200_stereo.wav`

### Requisitos de Almacenamiento

| Modo | Bitrate | Por Minuto | Por Hora |
|------|---------|------------|----------|
| Mixed (mono) | 128 kbps | ~1 MB | ~60 MB |
| Stereo | 256 kbps | ~2 MB | ~120 MB |
| Separate | 256 kbps | ~2 MB | ~120 MB |

### Politica de Retencion

Configurar limpieza automatica:

```json
{
  "recording": {
    "retention_days": 30
  }
}
```

Las grabaciones mas antiguas que `retention_days` se eliminan automaticamente.

### Almacenamiento en Red

Para despliegues de alta disponibilidad, monte almacenamiento en red:

```bash
# Montaje NFS
mount -t nfs storage.example.com:/recordings /var/lib/karl/recordings

# O en /etc/fstab
storage.example.com:/recordings /var/lib/karl/recordings nfs defaults 0 0
```

Para Kubernetes, use un PersistentVolumeClaim:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: karl-recordings
spec:
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 100Gi
  storageClassName: nfs
```

---

## Activar la Grabacion

### Metodo 1: Flag del Protocolo NG

Agregue el flag `record-call` al crear una sesion.

**OpenSIPS**:
```opensips
rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
```

**Kamailio**:
```kamailio
rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
```

### Metodo 2: API REST

Iniciar grabacion para una sesion activa:

```bash
# Iniciar grabacion
curl -X POST http://localhost:8080/api/v1/recording/start \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "abc123",
    "mode": "stereo"
  }'
```

Detener grabacion:

```bash
# Detener grabacion
curl -X POST http://localhost:8080/api/v1/recording/stop \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "abc123"
  }'
```

### Metodo 3: Grabar Todas las Llamadas

Configure la grabacion predeterminada en su proxy SIP:

**OpenSIPS** (grabar todas las llamadas):
```opensips
route {
    if (is_method("INVITE")) {
        rtpengine_manage("RTP/AVP replace-origin replace-session-connection record-call");
    }
}
```

### Metodo 4: Grabacion Condicional

Grabar basandose en criterios:

```opensips
route {
    if (is_method("INVITE")) {
        $var(flags) = "RTP/AVP replace-origin replace-session-connection";

        # Grabar llamadas de usuarios especificos
        if ($fU =~ "^sales_") {
            $var(flags) = $var(flags) + " record-call";
        }

        # Grabar llamadas a numeros especificos
        if ($rU =~ "^1800") {
            $var(flags) = $var(flags) + " record-call";
        }

        rtpengine_manage("$var(flags)");
    }
}
```

---

## Gestionar Grabaciones

### Listar Grabaciones

```bash
# Via API
curl http://localhost:8080/api/v1/recordings

# Respuesta
{
  "recordings": [
    {
      "id": "rec-001",
      "session_id": "abc123",
      "call_id": "call-abc123",
      "file_path": "/var/lib/karl/recordings/2024-01-15/call-abc123_1705312200_stereo.wav",
      "mode": "stereo",
      "duration_seconds": 125,
      "file_size": 3932160,
      "created_at": "2024-01-15T10:30:00Z"
    }
  ]
}
```

### Obtener Detalles de Grabacion

```bash
curl http://localhost:8080/api/v1/recordings/rec-001
```

### Descargar Grabacion

```bash
# Obtener ruta del archivo de la API, luego descargar
curl http://localhost:8080/api/v1/recordings/rec-001/download -o recording.wav
```

### Eliminar Grabacion

```bash
curl -X DELETE http://localhost:8080/api/v1/recordings/rec-001
```

### Listar Grabaciones por Fecha

```bash
# Listar grabaciones de una fecha especifica
ls -la /var/lib/karl/recordings/2024-01-15/

# Buscar grabaciones de una llamada especifica
find /var/lib/karl/recordings -name "*abc123*"
```

---

## Consideraciones de Produccion

### Planificacion de Almacenamiento

Calcular necesidades de almacenamiento:

```
Almacenamiento diario = llamadas_por_dia × duracion_promedio_minutos × MB_por_minuto

Ejemplo:
- 1000 llamadas/dia
- 5 minutos de duracion promedio
- 2 MB/minuto (stereo)
= 1000 × 5 × 2 = 10 GB/dia
= 300 GB/mes
```

### Alta Disponibilidad

1. **Almacenamiento compartido**: Use NFS, EFS o similar para despliegues multi-instancia
2. **Respaldo**: Respaldo regular del directorio de grabaciones
3. **Monitoreo**: Alerta sobre uso de espacio en disco

### Cumplimiento Normativo

Para industrias reguladas:

1. **Cifrado en reposo**: Use sistema de archivos cifrado
2. **Control de acceso**: Restringir permisos del directorio de grabaciones
3. **Registro de auditoria**: Rastrear acceso a grabaciones
4. **Retencion**: Configurar periodo de retencion apropiado

### Rendimiento

Impacto de la grabacion en el sistema:

| Grabaciones Concurrentes | CPU Adicional | Memoria Adicional |
|--------------------------|---------------|-------------------|
| 100 | ~5% | ~50 MB |
| 500 | ~15% | ~200 MB |
| 1000 | ~25% | ~400 MB |

---

## Solucion de Problemas

### Las Grabaciones No Se Crean

1. **Verificar que la grabacion esta habilitada**:
```bash
grep -A 10 '"recording"' /etc/karl/config.json
```

2. **Verificar permisos del directorio**:
```bash
ls -la /var/lib/karl/recordings
# Debe ser escribible por el usuario karl
```

3. **Verificar espacio en disco**:
```bash
df -h /var/lib/karl/recordings
```

4. **Verificar que se envia el flag record-call**:
```bash
# Habilitar logging de depuracion
KARL_LOG_LEVEL=debug ./karl

# Buscar mensajes de grabacion
grep -i recording /var/log/karl.log
```

### Archivos de Grabacion Vacios

1. **Verificar que la sesion tiene medios**:
```bash
curl http://localhost:8080/api/v1/sessions/abc123
# Verificar packets_received > 0
```

2. **Verificar soporte de codecs**:
Karl soporta G.711 (PCMU/PCMA) y Opus. Otros codecs pueden no grabarse correctamente.

3. **Verificar errores**:
```bash
grep -i "recording\|codec" /var/log/karl.log
```

### Mala Calidad de Grabacion

1. **Desajuste de tasa de muestreo**: Asegurese de que sample_rate coincida con su codec
2. **Problemas de red**: La alta perdida de paquetes afecta la calidad de grabacion
3. **Jitter**: Habilite el jitter buffer para mejor calidad

### Problemas de Espacio en Disco

1. **Habilitar politica de retencion**:
```json
{
  "recording": {
    "retention_days": 7
  }
}
```

2. **Limpieza manual**:
```bash
# Eliminar grabaciones con mas de 7 dias
find /var/lib/karl/recordings -name "*.wav" -mtime +7 -delete
```

3. **Monitorear uso de disco**:
```bash
# Agregar a cron
0 * * * * /usr/bin/df -h /var/lib/karl/recordings | mail -s "Karl disk usage" admin@example.com
```

---

## Siguientes Pasos

- [Monitorear con Prometheus](./monitoring-prometheus.md)
- [Integrar con OpenSIPS](./integrating-opensips.md)
- [Integrar con Kamailio](./integrating-kamailio.md)
