[English](../README.md) | [中文](../zh-CN/README.md) | [Français](../fr/README.md) | **Español**

# Documentacion del Servidor de Medios Karl

Bienvenido a la documentacion del Servidor de Medios Karl. Esta guia le ayudara a instalar, configurar y operar Karl en entornos de produccion.

## Resumen de la Documentacion

### Primeros Pasos

- [Guia de Inicio Rapido](./getting-started.md) - Ponga Karl en funcionamiento en 5 minutos
- [Guia de Instalacion](./installation.md) - Instrucciones detalladas de instalacion
- [Referencia de Configuracion](./configuration.md) - Opciones completas de configuracion

### Guias Practicas

Guias paso a paso para tareas comunes:

- [Desplegar en Kubernetes](./how-to/deploying-kubernetes.md) - Ejecutar Karl en clusters de Kubernetes
- [Integrar con OpenSIPS](./how-to/integrating-opensips.md) - Conectar Karl a OpenSIPS
- [Integrar con Kamailio](./how-to/integrating-kamailio.md) - Conectar Karl a Kamailio
- [Configurar Grabacion de Llamadas](./how-to/setting-up-recording.md) - Habilitar y configurar la grabacion de llamadas
- [Monitorear con Prometheus](./how-to/monitoring-prometheus.md) - Configurar metricas y alertas
- [Escalar Horizontalmente](./how-to/scaling-horizontally.md) - Ejecutar multiples instancias de Karl
- [Puente WebRTC a SIP](./how-to/webrtc-sip-bridging.md) - Conectar clientes de navegador a redes SIP
- [Asegurar con TLS](./how-to/securing-with-tls.md) - Habilitar cifrado para la API y el plano de control
- [Guia de Solucion de Problemas](./how-to/troubleshooting.md) - Diagnosticar y resolver problemas comunes

### Referencia

- [Referencia del Protocolo NG](./reference/ng-protocol.md) - Documentacion completa del protocolo NG
- [Referencia de la API REST](./reference/rest-api.md) - Endpoints de la API y ejemplos
- [Referencia de Metricas](./reference/metrics.md) - Documentacion de metricas de Prometheus
- [Variables de Entorno](./reference/environment-variables.md) - Todas las variables de entorno soportadas

### Arquitectura

- [Resumen de Arquitectura](./architecture/overview.md) - Como funciona Karl internamente
- [Ciclo de Vida de Sesion](./architecture/session-lifecycle.md) - Flujo de llamada y gestion de estado
- [Procesamiento de Medios](./architecture/media-processing.md) - Manejo de RTP y funciones de calidad

---

## Enlaces Rapidos

| Tarea | Documentacion |
|-------|---------------|
| Instalar Karl | [Guia de Instalacion](./installation.md) |
| Configurar para produccion | [Referencia de Configuracion](./configuration.md) |
| Desplegar en Kubernetes | [Guia de Kubernetes](./how-to/deploying-kubernetes.md) |
| Configurar monitoreo | [Guia de Prometheus](./how-to/monitoring-prometheus.md) |
| Depurar problemas | [Guia de Solucion de Problemas](./how-to/troubleshooting.md) |

---

## Soporte

- **GitHub Issues**: [Reportar errores y solicitar funcionalidades](https://github.com/loreste/karl/issues)
- **GitHub Discussions**: [Hacer preguntas y compartir ideas](https://github.com/loreste/karl/discussions)
