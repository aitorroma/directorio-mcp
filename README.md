<p align="center"><img src="assets/logo.svg" width="96" alt="Directorio MCP"></p>

<h1 align="center">Directorio MCP</h1>

<p align="center">Servidores <a href="https://modelcontextprotocol.io">MCP</a> remotos, revisados y en español, para conectar a tu asistente.<br>
<a href="https://aitorroma.github.io/directorio-mcp/">Ver el directorio</a> · <a href="https://aitorroma.github.io/directorio-mcp/index.json">index.json</a> · <a href="https://github.com/aitorroma/directorio-mcp/issues/new?template=nuevo-servidor.yml">Proponer un servidor</a></p>

---

Ya existe un [registro oficial de MCP](https://registry.modelcontextprotocol.io), pero es abierto a todo, casi todo en inglés y la mayoría son paquetes para instalar. Este directorio es otra cosa:

- **Solo servidores remotos**: una URL `https://` a la que conectarse, sin instalar nada.
- **Revisados**: cada ficha la valida un programa y alguien la aprueba; una vez por semana se comprueba que todos siguen respondiendo.
- **En español**: la descripción y las categorías, pensadas para quien no programa.
- **Con lo que importa a la vista**: cuántas herramientas tiene, si solo leen o también cambian cosas, y si pide clave.

Nació para nimsting, un asistente personal que añade servidores desde aquí con un clic, pero el índice es público: cualquier asistente o cliente MCP puede leerlo.

## Añadir un servidor

**Sin tocar código**: rellena el [formulario](https://github.com/aitorroma/directorio-mcp/issues/new?template=nuevo-servidor.yml) y lo añadimos.

**Con un pull request**:

1. Crea `servidores/<id>.json` (el `id` en minúsculas, números y guiones):

   ```json
   {
     "nombre": "Precios de gasolina",
     "url": "https://mcp.preciosgasolina.es/mcp",
     "descripcion": "Precios oficiales de las gasolineras de España…",
     "categoria": "motor",
     "etiquetas": ["gasolina", "diésel"],
     "autenticacion": "ninguna",
     "web": "https://preciosgasolina.es/desarrolladores/mcp/",
     "idiomas": ["es"],
     "pais": "ES"
   }
   ```

2. Si quieres, añade su logo en `logos/<id>.svg` (o `.png`): cuadrado, SVG de 50 KB como mucho (sin scripts, eventos ni enlaces externos) o PNG de 100 KB.
3. Compruébalo: `go run ./cmd/comprobar -probar servidores/<id>.json`.
4. Abre el pull request. Las Actions validan la ficha y se conectan al servidor; si responde, se revisa y se acepta.

### Campos

| Campo | Obligatorio | Qué es |
|---|---|---|
| `nombre` | sí | Como se llama el servicio (60 caracteres como mucho) |
| `url` | sí | La URL MCP, `https://` (transporte HTTP «streamable») |
| `descripcion` | sí | Qué hace, en español, de 30 a 400 caracteres |
| `categoria` | sí | `motor`, `viajes`, `compras`, `finanzas`, `salud`, `hogar`, `trabajo`, `desarrollo`, `informacion`, `ocio`, `administracion` u `otros` |
| `autenticacion` | sí | `ninguna`, `cabecera` (una clave en una cabecera HTTP) u `oauth` |
| `cabecera` | con `cabecera` | El nombre de la cabecera: `Authorization`, `X-API-Key`… |
| `como_conseguir_clave` | no | Dónde se saca la clave |
| `etiquetas` | no | Hasta 10 palabras para buscarlo |
| `web` | no | La web del servicio o de su documentación |
| `idiomas`, `pais` | no | `["es", "en"]`, `"ES"` |

### Qué entra

- Servidores públicos o cuyo dueño permite listarlos.
- Que respondan como MCP (`initialize` y `tools/list`) en su URL.
- Nada que sirva para saltarse las condiciones de otros servicios, ni servidores de dudosa procedencia.

## El índice

`index.json` lleva cada ficha más lo comprobado: `estado` (`ok`, `necesita_clave`, `caido`), las herramientas con su descripción y si son de solo lectura, `solo_lectura` (todas lo son), `logo` (ruta relativa al índice) y la fecha de la comprobación. Se regenera con cada cambio y cada lunes.

## Licencia

MIT. Los logos son de sus dueños y están aquí solo para identificar cada servicio.
